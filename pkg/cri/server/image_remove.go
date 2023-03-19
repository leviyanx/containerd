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
	"fmt"
	"os"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"

	wasmmodule "github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemoveImage removes the image.
// TODO(random-liu): Update CRI to pass image reference instead of ImageSpec. (See
// kubernetes/kubernetes#46255)
// TODO(random-liu): We should change CRI to distinguish image id and image spec.
// Remove the whole image no matter the it's image id or reference. This is the
// semantic defined in CRI now.
func (c *criService) RemoveImage(ctx context.Context, r *runtime.RemoveImageRequest) (*runtime.RemoveImageResponse, error) {
	if wasmmodule.IsWasmModule(*r.GetImage()) {
		// find the module in store
		wasmModuleName := r.GetImage().GetImage()
		_, err := c.wasmModuleStore.Resolve(wasmModuleName)
		if err != nil {
			return nil, fmt.Errorf("there doesn't have the wasm module %q : %w", r.GetImage().GetImage(), err)
		}

		// TODO: put the function that delete the file in local disk into the store
		// delete the file in local disk
		wasmModule, err := c.wasmModuleStore.Get(wasmModuleName)
		if err != nil {
			return nil, fmt.Errorf("failed to get wasm module %q : %w", r.GetImage().GetImage(), err)
		}
		if _, err := c.os.Stat(wasmModule.Filepath); os.IsNotExist(err) {
			return nil, fmt.Errorf("the wasm module file %q doesn't exist : %w", wasmModule.Filepath, err)
		} else {
			err := c.os.RemoveAll(wasmModule.Filepath)
			if err != nil {
				return nil, fmt.Errorf("failed to delete the wasm module file %q : %w", wasmModule.Filepath, err)
			}
		}

		// delete the module and reference in store
		if err := c.wasmModuleStore.Delete(wasmModuleName); err != nil {
			return nil, fmt.Errorf("failed to delete wasm module %q : %w", r.GetImage().GetImage(), err)
		}
		return &runtime.RemoveImageResponse{}, nil
	}

	image, err := c.localResolve(r.GetImage().GetImage())
	if err != nil {
		if errdefs.IsNotFound(err) {
			// return empty without error when image not found.
			return &runtime.RemoveImageResponse{}, nil
		}
		return nil, fmt.Errorf("can not resolve %q locally: %w", r.GetImage().GetImage(), err)
	}

	// Remove all image references.
	for i, ref := range image.References {
		var opts []images.DeleteOpt
		if i == len(image.References)-1 {
			// Delete the last image reference synchronously to trigger garbage collection.
			// This is best effort. It is possible that the image reference is deleted by
			// someone else before this point.
			opts = []images.DeleteOpt{images.SynchronousDelete()}
		}
		err = c.client.ImageService().Delete(ctx, ref, opts...)
		if err == nil || errdefs.IsNotFound(err) {
			// Update image store to reflect the newest state in containerd.
			if err := c.imageStore.Update(ctx, ref); err != nil {
				return nil, fmt.Errorf("failed to update image reference %q for %q: %w", ref, image.ID, err)
			}
			continue
		}
		return nil, fmt.Errorf("failed to delete image reference %q for %q: %w", ref, image.ID, err)
	}
	return &runtime.RemoveImageResponse{}, nil
}
