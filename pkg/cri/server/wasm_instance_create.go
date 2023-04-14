package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cri/config"
	cio "github.com/containerd/containerd/pkg/cri/io"
	customopts "github.com/containerd/containerd/pkg/cri/opts"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"github.com/containerd/containerd/pkg/cri/util"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *criService) createWasmInstance(ctx context.Context, r *runtime.CreateContainerRequest) (_ *runtime.CreateContainerResponse, retErr error) {
	containerConfig := r.GetConfig()
	log.G(ctx).Debugf("Container config %+v", containerConfig)
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
	metadata := containerConfig.GetMetadata()
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
		Config:    containerConfig,
	}

	// get wasm module
	wasmModule, err := c.wasmModuleStore.Get(containerConfig.GetImage().GetImage())
	if err != nil {
		return nil, fmt.Errorf("failed to find wasm module %q: %w", containerConfig.GetImage().GetImage(), err)
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

	spec, err := c.wasmSpec(ctx, id, &wasmModule, containerConfig, sandboxConfig, wasmRuntime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate wasm %q spec: %w", id, err)
	}

	// TODO: handle any KVM based runtime

	// NOTE: don't create snapshotter

	// TODO: get stopSignal

	// Validate log paths and compose full log paths.
	if sandboxConfig.GetLogDirectory() != "" && containerConfig.GetLogPath() != "" {
		meta.LogPath = filepath.Join(sandboxConfig.GetLogDirectory(), containerConfig.GetLogPath())
	} else {
		log.G(ctx).Infof("Logging will be disabled due to empty log paths for sandbox (%q) or wasm instance (%q)",
			sandboxConfig.GetLogDirectory(), containerConfig.GetLogPath())
	}

	// Create wasm instance IO
	wasmInstanceIO, err := cio.NewContainerIO(id,
		cio.WithNewFIFOs(volatileWasmInstanceRootDir, containerConfig.GetTty(), containerConfig.GetStdin()))
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
	wasmInstanceLabels := buildLabels(containerConfig.Labels, make(map[string]string), "wasm instance")
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
func (c *criService) wasmSpec(
  ctx context.Context,
  id string,
  wasmModule *wasmmodule.WasmModule,
	config *runtime.ContainerConfig,
  sandboxConfig *runtime.PodSandboxConfig,
  ociRuntime config.Runtime,
) (*runtimespec.Spec, error) {

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

  // set command
	specOpts = append(specOpts, withProcessArgs(wasmModule, config))

	// mount the wasm file path from host path to container path '/'
	var volumeMounts = []*runtime.Mount{
		{
			ContainerPath: "/",
			HostPath:      filepath.Dir(wasmModule.GetFilepath()),
		},
	}

	// use empty container config and selinux label.to get necessary work done
	specOpts = append(specOpts, customopts.WithMounts(c.os, &runtime.ContainerConfig{}, volumeMounts, ""))
	return oci.GenerateSpec(ctx, nil, &containers.Container{ID: id}, specOpts...)
}

func withProcessArgs(wasmModule *wasmmodule.WasmModule, config *runtime.ContainerConfig) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *runtimespec.Spec) (err error) {
		command, args := config.GetCommand(), config.GetArgs()
    if command == nil || len(command) == 0 {
      filename := filepath.Base(wasmModule.GetFilepath())
      return oci.WithProcessArgs(filename)(ctx, client, c, s)
    }
    return oci.WithProcessArgs(append(command, args...)...)(ctx, client, c, s)
  }
}
