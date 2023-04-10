/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package server

import (
	"errors"
	"fmt"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"path/filepath"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/typeurl"
	"github.com/davecgh/go-spew/spew"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	cio "github.com/containerd/containerd/pkg/cri/io"
	customopts "github.com/containerd/containerd/pkg/cri/opts"
	containerstore "github.com/containerd/containerd/pkg/cri/store/container"
	wasminstance "github.com/containerd/containerd/pkg/cri/store/wasminstance"
	"github.com/containerd/containerd/pkg/cri/config"
	"github.com/containerd/containerd/pkg/cri/util"
	ctrdutil "github.com/containerd/containerd/pkg/cri/util"
)

func init() {
	typeurl.Register(&containerstore.Metadata{},
		"github.com/containerd/cri/pkg/store/container", "Metadata")
}

// CreateContainer creates a new container in the given PodSandbox.
func (c *criService) CreateContainer(ctx context.Context, r *runtime.CreateContainerRequest) (_ *runtime.CreateContainerResponse, retErr error) {
	config := r.GetConfig()

	// when the image is wasm module, create a wasm instance instead of a container
	if wasmmodule.IsWasmModule(config.GetImage()) {
		return c.createWasmInstance(ctx, r)
	}

	log.G(ctx).Debugf("Container config %+v", config)
	sandboxConfig := r.GetSandboxConfig()
	sandbox, err := c.sandboxStore.Get(r.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("failed to find sandbox id %q: %w", r.GetPodSandboxId(), err)
	}
	sandboxID := sandbox.ID
	s, err := sandbox.Container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox container task: %w", err)
	}
	sandboxPid := s.Pid()

	// Generate unique id and name for the container and reserve the name.
	// Reserve the container name to avoid concurrent `CreateContainer` request creating
	// the same container.
	id := util.GenerateID()
	metadata := config.GetMetadata()
	if metadata == nil {
		return nil, errors.New("container config must include metadata")
	}
	containerName := metadata.Name
	name := makeContainerName(metadata, sandboxConfig.GetMetadata())
	log.G(ctx).Debugf("Generated id %q for container %q", id, name)
	if err = c.containerNameIndex.Reserve(name, id); err != nil {
		return nil, fmt.Errorf("failed to reserve container name %q: %w", name, err)
	}
	defer func() {
		// Release the name if the function returns with an error.
		if retErr != nil {
			c.containerNameIndex.ReleaseByName(name)
		}
	}()

	// Create initial internal container metadata.
	meta := containerstore.Metadata{
		ID:        id,
		Name:      name,
		SandboxID: sandboxID,
		Config:    config,
	}

	// Prepare container image snapshot. For container, the image should have
	// been pulled before creating the container, so do not ensure the image.
	image, err := c.localResolve(config.GetImage().GetImage())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image %q: %w", config.GetImage().GetImage(), err)
	}
	containerdImage, err := c.toContainerdImage(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to get image from containerd %q: %w", image.ID, err)
	}

	start := time.Now()
	// Run container using the same runtime with sandbox.
	sandboxInfo, err := sandbox.Container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox %q info: %w", sandboxID, err)
	}

	// Create container root directory.
	containerRootDir := c.getContainerRootDir(id)
	if err = c.os.MkdirAll(containerRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create container root directory %q: %w",
			containerRootDir, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup the container root directory.
			if err = c.os.RemoveAll(containerRootDir); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to remove container root directory %q",
					containerRootDir)
			}
		}
	}()
	volatileContainerRootDir := c.getVolatileContainerRootDir(id)
	if err = c.os.MkdirAll(volatileContainerRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create volatile container root directory %q: %w",
			volatileContainerRootDir, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup the volatile container root directory.
			if err = c.os.RemoveAll(volatileContainerRootDir); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to remove volatile container root directory %q",
					volatileContainerRootDir)
			}
		}
	}()

	var volumeMounts []*runtime.Mount
	if !c.config.IgnoreImageDefinedVolumes {
		// Create container image volumes mounts.
		volumeMounts = c.volumeMounts(containerRootDir, config.GetMounts(), &image.ImageSpec.Config)
	} else if len(image.ImageSpec.Config.Volumes) != 0 {
		log.G(ctx).Debugf("Ignoring volumes defined in image %v because IgnoreImageDefinedVolumes is set", image.ID)
	}

	// Generate container mounts.
	mounts := c.containerMounts(sandboxID, config)

	ociRuntime, err := c.getSandboxRuntime(sandboxConfig, sandbox.Metadata.RuntimeHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox runtime: %w", err)
	}
	log.G(ctx).Debugf("Use OCI runtime %+v for sandbox %q and container %q", ociRuntime, sandboxID, id)

	spec, err := c.containerSpec(id, sandboxID, sandboxPid, sandbox.NetNSPath, containerName, containerdImage.Name(), config, sandboxConfig,
		&image.ImageSpec.Config, append(mounts, volumeMounts...), ociRuntime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate container %q spec: %w", id, err)
	}

	meta.ProcessLabel = spec.Process.SelinuxLabel

	// handle any KVM based runtime
	if err := modifyProcessLabel(ociRuntime.Type, spec); err != nil {
		return nil, err
	}

	if config.GetLinux().GetSecurityContext().GetPrivileged() {
		// If privileged don't set the SELinux label but still record it on the container so
		// the unused MCS label can be release later
		spec.Process.SelinuxLabel = ""
	}
	defer func() {
		if retErr != nil {
			selinux.ReleaseLabel(spec.Process.SelinuxLabel)
		}
	}()

	log.G(ctx).Debugf("Container %q spec: %#+v", id, spew.NewFormatter(spec))

	snapshotterOpt := snapshots.WithLabels(snapshots.FilterInheritedLabels(config.Annotations))
	// Set snapshotter before any other options.
	opts := []containerd.NewContainerOpts{
		containerd.WithSnapshotter(c.config.ContainerdConfig.Snapshotter),
		// Prepare container rootfs. This is always writeable even if
		// the container wants a readonly rootfs since we want to give
		// the runtime (runc) a chance to modify (e.g. to create mount
		// points corresponding to spec.Mounts) before making the
		// rootfs readonly (requested by spec.Root.Readonly).
		customopts.WithNewSnapshot(id, containerdImage, snapshotterOpt),
	}
	if len(volumeMounts) > 0 {
		mountMap := make(map[string]string)
		for _, v := range volumeMounts {
			mountMap[filepath.Clean(v.HostPath)] = v.ContainerPath
		}
		opts = append(opts, customopts.WithVolumes(mountMap))
	}
	meta.ImageRef = image.ID
	meta.StopSignal = image.ImageSpec.Config.StopSignal

	// Validate log paths and compose full container log path.
	if sandboxConfig.GetLogDirectory() != "" && config.GetLogPath() != "" {
		meta.LogPath = filepath.Join(sandboxConfig.GetLogDirectory(), config.GetLogPath())
		log.G(ctx).Debugf("Composed container full log path %q using sandbox log dir %q and container log path %q",
			meta.LogPath, sandboxConfig.GetLogDirectory(), config.GetLogPath())
	} else {
		log.G(ctx).Infof("Logging will be disabled due to empty log paths for sandbox (%q) or container (%q)",
			sandboxConfig.GetLogDirectory(), config.GetLogPath())
	}

	containerIO, err := cio.NewContainerIO(id,
		cio.WithNewFIFOs(volatileContainerRootDir, config.GetTty(), config.GetStdin()))
	if err != nil {
		return nil, fmt.Errorf("failed to create container io: %w", err)
	}
	defer func() {
		if retErr != nil {
			if err := containerIO.Close(); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to close container io %q", id)
			}
		}
	}()

	specOpts, err := c.containerSpecOpts(config, &image.ImageSpec.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to get container spec opts: %w", err)
	}

	containerLabels := buildLabels(config.Labels, image.ImageSpec.Config.Labels, containerKindContainer)

	runtimeOptions, err := getRuntimeOptions(sandboxInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime options: %w", err)
	}
	opts = append(opts,
		containerd.WithSpec(spec, specOpts...),
		containerd.WithRuntime(sandboxInfo.Runtime.Name, runtimeOptions),
		containerd.WithContainerLabels(containerLabels),
		containerd.WithContainerExtension(containerMetadataExtension, &meta))
	var cntr containerd.Container
	if cntr, err = c.client.NewContainer(ctx, id, opts...); err != nil {
		return nil, fmt.Errorf("failed to create containerd container: %w", err)
	}
	defer func() {
		if retErr != nil {
			deferCtx, deferCancel := ctrdutil.DeferContext()
			defer deferCancel()
			if err := cntr.Delete(deferCtx, containerd.WithSnapshotCleanup); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to delete containerd container %q", id)
			}
		}
	}()

	status := containerstore.Status{CreatedAt: time.Now().UnixNano()}
	status = copyResourcesToStatus(spec, status)
	container, err := containerstore.NewContainer(meta,
		containerstore.WithStatus(status, containerRootDir),
		containerstore.WithContainer(cntr),
		containerstore.WithContainerIO(containerIO),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal container object for %q: %w", id, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup container checkpoint on error.
			if err := container.Delete(); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to cleanup container checkpoint for %q", id)
			}
		}
	}()

	// Add container into container store.
	if err := c.containerStore.Add(container); err != nil {
		return nil, fmt.Errorf("failed to add container %q into store: %w", id, err)
	}

	containerCreateTimer.WithValues(ociRuntime.Type).UpdateSince(start)

	return &runtime.CreateContainerResponse{ContainerId: id}, nil
}

// volumeMounts sets up image volumes for container. Rely on the removal of container
// root directory to do cleanup. Note that image volume will be skipped, if there is criMounts
// specified with the same destination.
func (c *criService) volumeMounts(containerRootDir string, criMounts []*runtime.Mount, config *imagespec.ImageConfig) []*runtime.Mount {
	if len(config.Volumes) == 0 {
		return nil
	}
	var mounts []*runtime.Mount
	for dst := range config.Volumes {
		if isInCRIMounts(dst, criMounts) {
			// Skip the image volume, if there is CRI defined volume mapping.
			// TODO(random-liu): This should be handled by Kubelet in the future.
			// Kubelet should decide what to use for image volume, and also de-duplicate
			// the image volume and user mounts.
			continue
		}
		volumeID := util.GenerateID()
		src := filepath.Join(containerRootDir, "volumes", volumeID)
		// addOCIBindMounts will create these volumes.
		mounts = append(mounts, &runtime.Mount{
			ContainerPath:  dst,
			HostPath:       src,
			SelinuxRelabel: true,
		})
	}
	return mounts
}

// runtimeSpec returns a default runtime spec used in cri-containerd.
func (c *criService) runtimeSpec(id string, baseSpecFile string, opts ...oci.SpecOpts) (*runtimespec.Spec, error) {
	// GenerateSpec needs namespace.
	ctx := ctrdutil.NamespacedContext()
	container := &containers.Container{ID: id}

	if baseSpecFile != "" {
		baseSpec, ok := c.baseOCISpecs[baseSpecFile]
		if !ok {
			return nil, fmt.Errorf("can't find base OCI spec %q", baseSpecFile)
		}

		spec := oci.Spec{}
		if err := util.DeepCopy(&spec, &baseSpec); err != nil {
			return nil, fmt.Errorf("failed to clone OCI spec: %w", err)
		}

		// Fix up cgroups path
		applyOpts := append([]oci.SpecOpts{oci.WithNamespacedCgroup()}, opts...)

		if err := oci.ApplyOpts(ctx, nil, container, &spec, applyOpts...); err != nil {
			return nil, fmt.Errorf("failed to apply OCI options: %w", err)
		}

		return &spec, nil
	}

	spec, err := oci.GenerateSpec(ctx, nil, container, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate spec: %w", err)
	}

	return spec, nil
}

func (c *criService) createWasmInstance(ctx context.Context, r *runtime.CreateContainerRequest) (_ *runtime.CreateContainerResponse, retErr error) {
	config := r.GetConfig()
	log.G(ctx).Debugf("Container config %+v", config)
	sandboxConfig := r.GetSandboxConfig()
	sandbox, err := c.sandboxStore.Get(r.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("failed to find sandbox id %q: %w", r.GetPodSandboxId(), err)
	}
	sandboxID := sandbox.ID
	// TODO: get sandbox container task when creating spec

	// Generate unique id and name for the wasm instance and reserve the name.
	// Reserve the wasm instance name to avoid concurrent `CreateContainer` request creating
	// the same wasm instance.
	id := util.GenerateID()
	metadata := config.GetMetadata()
	if metadata == nil {
		return nil, errors.New("container config must include metadata")
	}
	name := makeContainerName(metadata, sandboxConfig.GetMetadata())
	log.G(ctx).Debugf("Generated id %q for wasm instance %q", id, name)
	// TODO: use containerNameIndex instead, in case of id conflict
	if err = c.wasmInstanceNameIndex.Reserve(name, id); err != nil {
		return nil, fmt.Errorf("failed to reserve wasm instance name %q: %w", name, err)
	}
	defer func() {
		// Release the name if the function returns with an error.
		if retErr != nil {
			c.wasmInstanceNameIndex.ReleaseByName(name)
		}
	}()

	// Create initial internal wasm instance metadata.
	meta := wasminstance.Metadata{
		ID:        id,
		Name:      name,
		SandboxID: sandboxID,
		Config:    config,
	}

	// get wasm module
	wasmModule, err := c.wasmModuleStore.Get(config.GetImage().GetImage())
	if err != nil {
		return nil, fmt.Errorf("failed to find wasm module %q: %w", config.GetImage().GetImage(), err)
	}
	meta.WasmModuleName = wasmModule.Name

	start := time.Now()
	// Run wasm instance using the same runtime with sandbox
	sandboxInfo, err := sandbox.Container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox container info: %w", err)
	}

	// create wasm instance root directory (but without volatile directory)
	wasmInstanceRootDir := c.getWasmInstanceRootDir(id)
	if err = c.os.MkdirAll(wasmInstanceRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create wasm instance root directory %q: %w", wasmInstanceRootDir, err)
	}
	defer func() {
		if retErr != nil {
			if err = c.os.RemoveAll(wasmInstanceRootDir); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to remove wasm instance root directory %q", wasmInstanceRootDir)
			}
		}
	}()
	meta.WasmInstanceRootDir = wasmInstanceRootDir
	volatileWasmInstanceRootDir := c.getVolatileWasmInstanceRootDir(id)
	if err = c.os.MkdirAll(volatileWasmInstanceRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create volatile wasm instance root directory %q: %w", volatileWasmInstanceRootDir, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup the volatile wasm instance root directory.
			if err = c.os.RemoveAll(volatileWasmInstanceRootDir); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to remove volatile wasm instance root directory %q", volatileWasmInstanceRootDir)
			}
		}
	}()

	// NOTE: don't create wasm module volumes mounts
	// NOTE: don't generate wasm instance mounts

	// get wasm runtime
	wasmRuntime, err := c.getSandboxRuntime(sandboxConfig, sandbox.Metadata.RuntimeHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox runtime: %w", err)
	}
	log.G(ctx).Debugf("Using wasm runtime %+v  for sandbox %q and wasm instance %q", wasmRuntime, sandboxID, id)

  // mount the wasm file path from host path to container path '/'
	var volumeMounts = []*runtime.Mount {
    {
      ContainerPath: "/",
      HostPath: filepath.Dir(wasmModule.GetFilepath()),
    },
  }
  spec, err := c.wasmSpec(ctx, id, wasmModule.GetName(), sandboxConfig, wasmRuntime, volumeMounts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate wasm %q spec: %w", id, err)
	}

	// TODO: handle any KVM based runtime

	// NOTE: don't create snapshotter

	// TODO: get stopSignal

	// Validate log paths and compose full log paths.
	if sandboxConfig.GetLogDirectory() != "" && config.GetLogPath() != "" {
		meta.LogPath = filepath.Join(sandboxConfig.GetLogDirectory(), config.GetLogPath())
	} else {
		log.G(ctx).Infof("Logging will be disabled due to empty log paths for sandbox (%q) or wasm instance (%q)",
			sandboxConfig.GetLogDirectory(), config.GetLogPath())
	}

	// Create wasm instance IO
	wasmInstanceIO, err := cio.NewContainerIO(id,
		cio.WithNewFIFOs(volatileWasmInstanceRootDir, config.GetTty(), config.GetStdin()))
	if err != nil {
		return nil, fmt.Errorf("failed to create wasm instance IO: %w", err)
	}
	defer func() {
		if retErr != nil {
			if err = wasmInstanceIO.Close(); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to close wasm instance IO %q", id)
			}
		}
	}()

	// There are no labels that come from image config
	wasmInstanceLabels := buildLabels(config.Labels, make(map[string]string), "wasm instance")
	meta.Labels = wasmInstanceLabels

	runtimeOptions, err := getRuntimeOptions(sandboxInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime options: %w", err)
	}

	status := wasminstance.Status{CreatedAt: time.Now().UnixNano()}
	// TODO: copy spec to status

	// Initialize the wasmInstance
	// 1) Use the same runtime with sandbox
	wasmInstance, err := wasminstance.NewWasmInstance(ctx, meta, c.client,
    wasminstance.WithSpec(spec),
		wasminstance.WithRuntime(sandboxInfo.Runtime.Name, runtimeOptions),
		wasminstance.WithStatus(status, wasmInstanceRootDir),
		wasminstance.WithWasmModule(wasmModule),
		wasminstance.WithWasmInstanceIO(wasmInstanceIO),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create wasm instance for %q: %w", id, err)
	}
	defer func() {
		if retErr != nil {
			// Cleanup wasm instance checkpoint on error.
			if err := wasmInstance.Delete(); err != nil {
				log.G(ctx).WithError(err).Errorf("Failed to cleanup wasm instance checkpoint for %q", id)
			}
		}
	}()

	// add wasm instance into store
	if err := c.wasmInstanceStore.Add(wasmInstance); err != nil {
		return nil, fmt.Errorf("failed to add wasm instance %q into store: %w", id, err)
	}

	wasmInstanceCreateTimer.WithValues(wasmRuntime.Type).UpdateSince(start)

	return &runtime.CreateContainerResponse{ContainerId: id}, nil
}

// generate basic spec for wasm
func (c *criService) wasmSpec(ctx context.Context, id string, filename string, sandboxConfig *runtime.PodSandboxConfig, ociRuntime config.Runtime, extraMounts []*runtime.Mount) (*runtimespec.Spec, error) {
	specOpts := []oci.SpecOpts{
		oci.WithoutRunMount,
	}
	// only clear the default security settings if the runtime does not have a custom
	// base runtime spec spec.  Admins can use this functionality to define
	// default ulimits, seccomp, or other default settings.
	if ociRuntime.BaseRuntimeSpec == "" {
		specOpts = append(specOpts, customopts.WithoutDefaultSecuritySettings)
	}
  specOpts = append(specOpts,
		customopts.WithRelativeRoot(relativeRootfsPath),
		oci.WithDefaultPathEnv,
	)
	// Add HOSTNAME env.
	var (
		err      error
		hostname = sandboxConfig.GetHostname()
	)
	if hostname == "" {
		if hostname, err = c.os.Hostname(); err != nil {
			return nil, err
		}
	}
	specOpts = append(specOpts, oci.WithEnv([]string{hostnameEnv + "=" + hostname}))

  // set wasm filename as cmd
	specOpts = append(specOpts, oci.WithProcessArgs(filename))

  // use empty container config and selinux label.to get necessary work done
	specOpts = append(specOpts, customopts.WithMounts(c.os, &runtime.ContainerConfig{}, extraMounts, ""))
  return oci.GenerateSpec(ctx, nil, &containers.Container{ ID: id }, specOpts...)
}

