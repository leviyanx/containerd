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
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	containerdimages "github.com/containerd/containerd/images"
	"github.com/containerd/containerd/log"
	snpkg "github.com/containerd/containerd/pkg/snapshotters"
	distribution "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/imgcrypt"
	"github.com/containerd/imgcrypt/images/encryption"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	criconfig "github.com/containerd/containerd/pkg/cri/config"
	wasmmodule "github.com/containerd/containerd/pkg/cri/store/wasmmodule"
)

// For image management:
// 1) We have an in-memory metadata index to:
//   a. Maintain ImageID -> RepoTags, ImageID -> RepoDigset relationships; ImageID
//   is the digest of image config, which conforms to oci image spec.
//   b. Cache constant and useful information such as image chainID, config etc.
//   c. An image will be added into the in-memory metadata only when it's successfully
//   pulled and unpacked.
//
// 2) We use containerd image metadata store and content store:
//   a. To resolve image reference (digest/tag) locally. During pulling image, we
//   normalize the image reference provided by user, and put it into image metadata
//   store with resolved descriptor. For the other operations, if image id is provided,
//   we'll access the in-memory metadata index directly; if image reference is
//   provided, we'll normalize it, resolve it in containerd image metadata store
//   to get the image id.
//   b. As the backup of in-memory metadata in 1). During startup, the in-memory
//   metadata could be re-constructed from image metadata store + content store.
//
// Several problems with current approach:
// 1) An entry in containerd image metadata store doesn't mean a "READY" (successfully
// pulled and unpacked) image. E.g. during pulling, the client gets killed. In that case,
// if we saw an image without snapshots or with in-complete contents during startup,
// should we re-pull the image? Or should we remove the entry?
//
// yanxuean: We can't delete image directly, because we don't know if the image
// is pulled by us. There are resource leakage.
//
// 2) Containerd suggests user to add entry before pulling the image. However if
// an error occurs during the pulling, should we remove the entry from metadata
// store? Or should we leave it there until next startup (resource leakage)?
//
// 3) The cri plugin only exposes "READY" (successfully pulled and unpacked) images
// to the user, which are maintained in the in-memory metadata index. However, it's
// still possible that someone else removes the content or snapshot by-pass the cri plugin,
// how do we detect that and update the in-memory metadata correspondingly? Always
// check whether corresponding snapshot is ready when reporting image status?
//
// 4) Is the content important if we cached necessary information in-memory
// after we pull the image? How to manage the disk usage of contents? If some
// contents are missing but snapshots are ready, is the image still "READY"?

// PullImage pulls an image with authentication config.
func (c *criService) PullImage(ctx context.Context, r *runtime.PullImageRequest) (*runtime.PullImageResponse, error) {
	logrus.Infof("Image %q has wasm annotation: %+v", r.GetImage().GetImage(), r.GetImage().GetAnnotations())
	// Check whether the imageSpec contains a wasm related annotation.
	if IsWasm(r.GetImage().GetAnnotations()) {
		wasmModuleName := r.GetImage().GetImage()
		// check whether module already exists
		resolvedID, err := c.wasmModuleStore.Resolve(wasmModuleName)
		if err == nil {
			logrus.Warningf("image already exists, for now we ignore pulling.")
			return &runtime.PullImageResponse{ImageRef: resolvedID}, nil
		}

		wasmModuleUrl := r.GetImage().GetAnnotations()["wasm.module.url"]
		// filename is required, used to name the wasm module file
		wasmModuleFilename, filenameExist := r.GetImage().GetAnnotations()["wasm.module.filename"]
		if !filenameExist || wasmModuleFilename == "" {
			return nil, fmt.Errorf("wasm module filename is empty")
		}
		// stopSignal is optional, used to stop the wasm instance, if not specified, SIGKILL will be used
		stopSignal, stopSignalExist := r.GetImage().GetAnnotations()["wasm.module.stopSignal"]
		if !stopSignalExist || stopSignal == "" {
			logrus.Warningf("stop signal is not specified, use SIGKILL as default")
			stopSignal = "SIGKILL"
		}

		// download wasm module
		fetchWasmModuleFromUrl := func(url string) ([]byte, error) {
			resp, err := http.Get(url)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			return data, nil
		}

		wasmModuleFile, err := fetchWasmModuleFromUrl(wasmModuleUrl)
		if err != nil {
			return nil, fmt.Errorf("fail to download wasm module: %w", err)
		}

		// calculate size of wasm module
		wasmModuleSize := int64(len(wasmModuleFile))

		// generate wasm module id
		hasher := sha256.New()
		hasher.Write(wasmModuleFile)
		wasmModuleId := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

		// generate wasm module file path
		// wasm module path: /wasmmodules/<wasm module id>/<wasm module file name>, e.g. /wasmmodules/1a2b3c4d5e6f7g8h9i0j/hello_world.wasm
		// `/wasmmodules` store all wasm modules
		wasmModuleFilePath := filepath.Join("/wasmmodules", wasmModuleId, wasmModuleFilename)

		// create wasm module
		wasmModule := wasmmodule.WasmModule{
			ID:       wasmModuleId,
			Name:     wasmModuleName,
			Filepath: wasmModuleFilePath,
			Size:     wasmModuleSize,
			WasmModuleSpec: wasmmodule.WasmModuleSpec{
				URL:        wasmModuleUrl,
				StopSignal: stopSignal,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Save wasm to wasm store
		err = c.wasmModuleStore.Add(wasmModule)
		if err != nil {
			return nil, fmt.Errorf("fail to save wasm module: %w", err)
		}

		// store wasm module file in disk after succeed to save wasm module to store
		wasmModulePath := filepath.Dir(wasmModuleFilePath) // store all wasm module
		if _, err := c.os.Stat(wasmModulePath); os.IsNotExist(err) {
			err := c.os.MkdirAll(wasmModulePath, 0755)
			if err != nil {
				return nil, fmt.Errorf("fail to create wasm module path: %w", err)
			}
		}
		err = c.os.WriteFile(wasmModuleFilePath, wasmModuleFile, 0644)
		if err != nil {
			return nil, fmt.Errorf("fail to save wasm module in disk: %w", err)
		}

		log.G(ctx).Debugf("PullImage: wasm module %q with wasm module id %q", wasmModuleName, wasmModuleId)

		return &runtime.PullImageResponse{ImageRef: wasmModuleId}, nil
	}

	imageRef := r.GetImage().GetImage()
	namedRef, err := distribution.ParseDockerRef(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageRef, err)
	}
	ref := namedRef.String()
	if ref != imageRef {
		log.G(ctx).Debugf("PullImage using normalized image ref: %q", ref)
	}
	var (
		resolver = docker.NewResolver(docker.ResolverOptions{
			Headers: c.config.Registry.Headers,
			Hosts:   c.registryHosts(ctx, r.GetAuth()),
		})
		isSchema1    bool
		imageHandler containerdimages.HandlerFunc = func(_ context.Context,
			desc imagespec.Descriptor) ([]imagespec.Descriptor, error) {
			if desc.MediaType == containerdimages.MediaTypeDockerSchema1Manifest {
				isSchema1 = true
			}
			return nil, nil
		}
	)

	pullOpts := []containerd.RemoteOpt{
		containerd.WithSchema1Conversion,
		containerd.WithResolver(resolver),
		containerd.WithPullSnapshotter(c.config.ContainerdConfig.Snapshotter),
		containerd.WithPullUnpack,
		containerd.WithPullLabel(imageLabelKey, imageLabelValue),
		containerd.WithMaxConcurrentDownloads(c.config.MaxConcurrentDownloads),
		containerd.WithImageHandler(imageHandler),
		containerd.WithUnpackOpts([]containerd.UnpackOpt{
			containerd.WithUnpackDuplicationSuppressor(c.unpackDuplicationSuppressor),
		}),
	}

	pullOpts = append(pullOpts, c.encryptedImagesPullOpts()...)
	if !c.config.ContainerdConfig.DisableSnapshotAnnotations {
		pullOpts = append(pullOpts,
			containerd.WithImageHandlerWrapper(snpkg.AppendInfoHandlerWrapper(ref)))
	}

	if c.config.ContainerdConfig.DiscardUnpackedLayers {
		// Allows GC to clean layers up from the content store after unpacking
		pullOpts = append(pullOpts,
			containerd.WithChildLabelMap(containerdimages.ChildGCLabelsFilterLayers))
	}

	image, err := c.client.Pull(ctx, ref, pullOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull and unpack image %q: %w", ref, err)
	}

	configDesc, err := image.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("get image config descriptor: %w", err)
	}
	imageID := configDesc.Digest.String()

	repoDigest, repoTag := getRepoDigestAndTag(namedRef, image.Target().Digest, isSchema1)
	for _, r := range []string{imageID, repoTag, repoDigest} {
		if r == "" {
			continue
		}
		if err := c.createImageReference(ctx, r, image.Target()); err != nil {
			return nil, fmt.Errorf("failed to create image reference %q: %w", r, err)
		}
		// Update image store to reflect the newest state in containerd.
		// No need to use `updateImage`, because the image reference must
		// have been managed by the cri plugin.
		if err := c.imageStore.Update(ctx, r); err != nil {
			return nil, fmt.Errorf("failed to update image store %q: %w", r, err)
		}
	}

	log.G(ctx).Debugf("Pulled image %q with image id %q, repo tag %q, repo digest %q", imageRef, imageID,
		repoTag, repoDigest)
	// NOTE(random-liu): the actual state in containerd is the source of truth, even we maintain
	// in-memory image store, it's only for in-memory indexing. The image could be removed
	// by someone else anytime, before/during/after we create the metadata. We should always
	// check the actual state in containerd before using the image or returning status of the
	// image.
	return &runtime.PullImageResponse{ImageRef: imageID}, nil
}

// ParseAuth parses AuthConfig and returns username and password/secret required by containerd.
func ParseAuth(auth *runtime.AuthConfig, host string) (string, string, error) {
	if auth == nil {
		return "", "", nil
	}
	if auth.ServerAddress != "" {
		// Do not return the auth info when server address doesn't match.
		u, err := url.Parse(auth.ServerAddress)
		if err != nil {
			return "", "", fmt.Errorf("parse server address: %w", err)
		}
		if host != u.Host {
			return "", "", nil
		}
	}
	if auth.Username != "" {
		return auth.Username, auth.Password, nil
	}
	if auth.IdentityToken != "" {
		return "", auth.IdentityToken, nil
	}
	if auth.Auth != "" {
		decLen := base64.StdEncoding.DecodedLen(len(auth.Auth))
		decoded := make([]byte, decLen)
		_, err := base64.StdEncoding.Decode(decoded, []byte(auth.Auth))
		if err != nil {
			return "", "", err
		}
		fields := strings.SplitN(string(decoded), ":", 2)
		if len(fields) != 2 {
			return "", "", fmt.Errorf("invalid decoded auth: %q", decoded)
		}
		user, passwd := fields[0], fields[1]
		return user, strings.Trim(passwd, "\x00"), nil
	}
	// TODO(random-liu): Support RegistryToken.
	// An empty auth config is valid for anonymous registry
	return "", "", nil
}

// createImageReference creates image reference inside containerd image store.
// Note that because create and update are not finished in one transaction, there could be race. E.g.
// the image reference is deleted by someone else after create returns already exists, but before update
// happens.
func (c *criService) createImageReference(ctx context.Context, name string, desc imagespec.Descriptor) error {
	img := containerdimages.Image{
		Name:   name,
		Target: desc,
		// Add a label to indicate that the image is managed by the cri plugin.
		Labels: map[string]string{imageLabelKey: imageLabelValue},
	}
	// TODO(random-liu): Figure out which is the more performant sequence create then update or
	// update then create.
	oldImg, err := c.client.ImageService().Create(ctx, img)
	if err == nil || !errdefs.IsAlreadyExists(err) {
		return err
	}
	if oldImg.Target.Digest == img.Target.Digest && oldImg.Labels[imageLabelKey] == imageLabelValue {
		return nil
	}
	_, err = c.client.ImageService().Update(ctx, img, "target", "labels."+imageLabelKey)
	return err
}

// updateImage updates image store to reflect the newest state of an image reference
// in containerd. If the reference is not managed by the cri plugin, the function also
// generates necessary metadata for the image and make it managed.
func (c *criService) updateImage(ctx context.Context, r string) error {
	img, err := c.client.GetImage(ctx, r)
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("get image by reference: %w", err)
	}
	if err == nil && img.Labels()[imageLabelKey] != imageLabelValue {
		// Make sure the image has the image id as its unique
		// identifier that references the image in its lifetime.
		configDesc, err := img.Config(ctx)
		if err != nil {
			return fmt.Errorf("get image id: %w", err)
		}
		id := configDesc.Digest.String()
		if err := c.createImageReference(ctx, id, img.Target()); err != nil {
			return fmt.Errorf("create image id reference %q: %w", id, err)
		}
		if err := c.imageStore.Update(ctx, id); err != nil {
			return fmt.Errorf("update image store for %q: %w", id, err)
		}
		// The image id is ready, add the label to mark the image as managed.
		if err := c.createImageReference(ctx, r, img.Target()); err != nil {
			return fmt.Errorf("create managed label: %w", err)
		}
	}
	// If the image is not found, we should continue updating the cache,
	// so that the image can be removed from the cache.
	if err := c.imageStore.Update(ctx, r); err != nil {
		return fmt.Errorf("update image store for %q: %w", r, err)
	}
	return nil
}

// getTLSConfig returns a TLSConfig configured with a CA/Cert/Key specified by registryTLSConfig
func (c *criService) getTLSConfig(registryTLSConfig criconfig.TLSConfig) (*tls.Config, error) {
	var (
		tlsConfig = &tls.Config{}
		cert      tls.Certificate
		err       error
	)
	if registryTLSConfig.CertFile != "" && registryTLSConfig.KeyFile == "" {
		return nil, fmt.Errorf("cert file %q was specified, but no corresponding key file was specified", registryTLSConfig.CertFile)
	}
	if registryTLSConfig.CertFile == "" && registryTLSConfig.KeyFile != "" {
		return nil, fmt.Errorf("key file %q was specified, but no corresponding cert file was specified", registryTLSConfig.KeyFile)
	}
	if registryTLSConfig.CertFile != "" && registryTLSConfig.KeyFile != "" {
		cert, err = tls.LoadX509KeyPair(registryTLSConfig.CertFile, registryTLSConfig.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert file: %w", err)
		}
		if len(cert.Certificate) != 0 {
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		tlsConfig.BuildNameToCertificate() //nolint:staticcheck
	}

	if registryTLSConfig.CAFile != "" {
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to get system cert pool: %w", err)
		}
		caCert, err := os.ReadFile(registryTLSConfig.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA file: %w", err)
		}
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	tlsConfig.InsecureSkipVerify = registryTLSConfig.InsecureSkipVerify
	return tlsConfig, nil
}

func hostDirFromRoots(roots []string) func(string) (string, error) {
	rootfn := make([]func(string) (string, error), len(roots))
	for i := range roots {
		rootfn[i] = config.HostDirFromRoot(roots[i])
	}
	return func(host string) (dir string, err error) {
		for _, fn := range rootfn {
			dir, err = fn(host)
			if (err != nil && !errdefs.IsNotFound(err)) || (dir != "") {
				break
			}
		}
		return
	}
}

// registryHosts is the registry hosts to be used by the resolver.
func (c *criService) registryHosts(ctx context.Context, auth *runtime.AuthConfig) docker.RegistryHosts {
	paths := filepath.SplitList(c.config.Registry.ConfigPath)
	if len(paths) > 0 {
		hostOptions := config.HostOptions{}
		hostOptions.Credentials = func(host string) (string, string, error) {
			hostauth := auth
			if hostauth == nil {
				config := c.config.Registry.Configs[host]
				if config.Auth != nil {
					hostauth = toRuntimeAuthConfig(*config.Auth)
				}
			}
			return ParseAuth(hostauth, host)
		}
		hostOptions.HostDir = hostDirFromRoots(paths)

		return config.ConfigureHosts(ctx, hostOptions)
	}

	return func(host string) ([]docker.RegistryHost, error) {
		var registries []docker.RegistryHost

		endpoints, err := c.registryEndpoints(host)
		if err != nil {
			return nil, fmt.Errorf("get registry endpoints: %w", err)
		}
		for _, e := range endpoints {
			u, err := url.Parse(e)
			if err != nil {
				return nil, fmt.Errorf("parse registry endpoint %q from mirrors: %w", e, err)
			}

			var (
				transport = newTransport()
				client    = &http.Client{Transport: transport}
				config    = c.config.Registry.Configs[u.Host]
			)

			if config.TLS != nil {
				transport.TLSClientConfig, err = c.getTLSConfig(*config.TLS)
				if err != nil {
					return nil, fmt.Errorf("get TLSConfig for registry %q: %w", e, err)
				}
			} else if docker.IsLocalhost(host) && u.Scheme == "http" {
				// Skipping TLS verification for localhost
				transport.TLSClientConfig = &tls.Config{
					InsecureSkipVerify: true,
				}
			}

			// Make a copy of `auth`, so that different authorizers would not reference
			// the same auth variable.
			auth := auth
			if auth == nil && config.Auth != nil {
				auth = toRuntimeAuthConfig(*config.Auth)
			}
			authorizer := docker.NewDockerAuthorizer(
				docker.WithAuthClient(client),
				docker.WithAuthCreds(func(host string) (string, string, error) {
					return ParseAuth(auth, host)
				}))

			if u.Path == "" {
				u.Path = "/v2"
			}

			registries = append(registries, docker.RegistryHost{
				Client:       client,
				Authorizer:   authorizer,
				Host:         u.Host,
				Scheme:       u.Scheme,
				Path:         u.Path,
				Capabilities: docker.HostCapabilityResolve | docker.HostCapabilityPull,
			})
		}
		return registries, nil
	}
}

// defaultScheme returns the default scheme for a registry host.
func defaultScheme(host string) string {
	if docker.IsLocalhost(host) {
		return "http"
	}
	return "https"
}

// addDefaultScheme returns the endpoint with default scheme
func addDefaultScheme(endpoint string) (string, error) {
	if strings.Contains(endpoint, "://") {
		return endpoint, nil
	}
	ue := "dummy://" + endpoint
	u, err := url.Parse(ue)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s", defaultScheme(u.Host), endpoint), nil
}

// registryEndpoints returns endpoints for a given host.
// It adds default registry endpoint if it does not exist in the passed-in endpoint list.
// It also supports wildcard host matching with `*`.
func (c *criService) registryEndpoints(host string) ([]string, error) {
	var endpoints []string
	_, ok := c.config.Registry.Mirrors[host]
	if ok {
		endpoints = c.config.Registry.Mirrors[host].Endpoints
	} else {
		endpoints = c.config.Registry.Mirrors["*"].Endpoints
	}
	defaultHost, err := docker.DefaultHost(host)
	if err != nil {
		return nil, fmt.Errorf("get default host: %w", err)
	}
	for i := range endpoints {
		en, err := addDefaultScheme(endpoints[i])
		if err != nil {
			return nil, fmt.Errorf("parse endpoint url: %w", err)
		}
		endpoints[i] = en
	}
	for _, e := range endpoints {
		u, err := url.Parse(e)
		if err != nil {
			return nil, fmt.Errorf("parse endpoint url: %w", err)
		}
		if u.Host == host {
			// Do not add default if the endpoint already exists.
			return endpoints, nil
		}
	}
	return append(endpoints, defaultScheme(defaultHost)+"://"+defaultHost), nil
}

// newTransport returns a new HTTP transport used to pull image.
// TODO(random-liu): Create a library and share this code with `ctr`.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:       30 * time.Second,
			KeepAlive:     30 * time.Second,
			FallbackDelay: 300 * time.Millisecond,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
	}
}

// encryptedImagesPullOpts returns the necessary list of pull options required
// for decryption of encrypted images based on the cri decryption configuration.
func (c *criService) encryptedImagesPullOpts() []containerd.RemoteOpt {
	if c.config.ImageDecryption.KeyModel == criconfig.KeyModelNode {
		ltdd := imgcrypt.Payload{}
		decUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&ltdd))
		opt := containerd.WithUnpackOpts([]containerd.UnpackOpt{decUnpackOpt})
		return []containerd.RemoteOpt{opt}
	}
	return nil
}
