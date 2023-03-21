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
	"encoding/json"
	"fmt"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	imagestore "github.com/containerd/containerd/pkg/cri/store/image"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ImageStatus returns the status of the image, returns nil if the image isn't present.
// TODO(random-liu): We should change CRI to distinguish image id and image spec. (See
// kubernetes/kubernetes#46255)
func (c *criService) ImageStatus(ctx context.Context, r *runtime.ImageStatusRequest) (*runtime.ImageStatusResponse, error) {
	if wasmModule, err := c.wasmModuleStore.Get(r.GetImage().GetImage()); err == nil {
		// when can find the wasm module in store
		runtimeImage := wasmToCRIImage(wasmModule)
		info, err := c.wasmToCRIImageInfo(ctx, &wasmModule, r.GetVerbose())
		if err != nil {
			return nil, fmt.Errorf("failed to generate wasm info: %w", err)
		}

		return &runtime.ImageStatusResponse{
			Image: runtimeImage,
			Info:  info,
		}, nil

	}

	image, err := c.localResolve(r.GetImage().GetImage())
	if err != nil {
		if errdefs.IsNotFound(err) {
			// return empty without error when image not found.
			return &runtime.ImageStatusResponse{}, nil
		}
		return nil, fmt.Errorf("can not resolve %q locally: %w", r.GetImage().GetImage(), err)
	}
	// TODO(random-liu): [P0] Make sure corresponding snapshot exists. What if snapshot
	// doesn't exist?

	runtimeImage := toCRIImage(image)
	info, err := c.toCRIImageInfo(ctx, &image, r.GetVerbose())
	if err != nil {
		return nil, fmt.Errorf("failed to generate image info: %w", err)
	}

	return &runtime.ImageStatusResponse{
		Image: runtimeImage,
		Info:  info,
	}, nil
}

// toCRIImage converts internal image object to CRI runtime.Image.
func toCRIImage(image imagestore.Image) *runtime.Image {
	repoTags, repoDigests := parseImageReferences(image.References)
	runtimeImage := &runtime.Image{
		Id:          image.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
		Size_:       uint64(image.Size),
	}
	uid, username := getUserFromImage(image.ImageSpec.Config.User)
	if uid != nil {
		runtimeImage.Uid = &runtime.Int64Value{Value: *uid}
	}
	runtimeImage.Username = username

	return runtimeImage
}

func wasmToCRIImage(wasmModule wasmmodule.WasmModule) *runtime.Image {
	runtimeImage := &runtime.Image{
		Id:          wasmModule.ID,
		RepoTags:    []string{wasmModule.ID},
		RepoDigests: []string{wasmModule.ID},
		Size_:       uint64(wasmModule.Size),
	}

	// no user(uid, username) in wasm module
	user := ""
	uid, username := getUserFromImage(user)
	if uid != nil {
		runtimeImage.Uid = &runtime.Int64Value{Value: *uid}
	}
	runtimeImage.Username = username

	return runtimeImage
}

// TODO (mikebrow): discuss moving this struct and / or constants for info map for some or all of these fields to CRI
type verboseImageInfo struct {
	ChainID   string          `json:"chainID"`
	ImageSpec imagespec.Image `json:"imageSpec"`
}

// toCRIImageInfo converts internal image object information to CRI image status response info map.
func (c *criService) toCRIImageInfo(ctx context.Context, image *imagestore.Image, verbose bool) (map[string]string, error) {
	if !verbose {
		return nil, nil
	}

	info := make(map[string]string)

	imi := &verboseImageInfo{
		ChainID:   image.ChainID,
		ImageSpec: image.ImageSpec,
	}

	m, err := json.Marshal(imi)
	if err == nil {
		info["info"] = string(m)
	} else {
		log.G(ctx).WithError(err).Errorf("failed to marshal info %v", imi)
		info["info"] = err.Error()
	}

	return info, nil
}

func (c *criService) wasmToCRIImageInfo(ctx context.Context, wasmModule *wasmmodule.WasmModule, verbose bool) (map[string]string, error) {
	if !verbose {
		return nil, nil
	}

	info := make(map[string]string)

	imi := &verboseImageInfo{
		ChainID: "wasm-has-no-chain-id",
		ImageSpec: imagespec.Image{
			Created: &wasmModule.CreatedAt,
		},
	}

	m, err := json.Marshal(imi)
	if err == nil {
		info["info"] = string(m)
	} else {
		log.G(ctx).WithError(err).Errorf("failed to marshal info %v", imi)
		info["info"] = err.Error()
	}

	return info, nil
}
