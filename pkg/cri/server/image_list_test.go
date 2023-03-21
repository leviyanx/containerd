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
	"testing"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	imagestore "github.com/containerd/containerd/pkg/cri/store/image"
	wasmstore "github.com/containerd/containerd/pkg/cri/store/wasmmodule"
)

func TestListWasmModulesList(t *testing.T) {
	c := newTestCRIService()
	wasmsInStore := []wasmstore.WasmModule{
		{
			ID:       "fp051QR9RWgnkrmRlNAldEU9pWD_RyLdVG0stfDImoQ=1234",
			Name:     "test-wasm-1",
			Filepath: "test-wasm-1.wasm",
			Size:     1000,
		},
		{
			ID:       "alskdjfalskdjfalksdjfklasjdfljasdkfj",
			Name:     "test-wasm-2",
			Filepath: "test-wasm-2.wasm",
			Size:     2000,
		},
	}
	expect := []*runtime.Image{
		{
			Id:          "fp051QR9RWgnkrmRlNAldEU9pWD_RyLdVG0stfDImoQ=1234",
			RepoTags:    []string{"fp051QR9RWgnkrmRlNAldEU9pWD_RyLdVG0stfDImoQ=1234"},
			RepoDigests: []string{"fp051QR9RWgnkrmRlNAldEU9pWD_RyLdVG0stfDImoQ=1234"},
			Size_:       uint64(1000),
			Username:    "",
		},
		{
			Id:          "alskdjfalskdjfalksdjfklasjdfljasdkfj",
			RepoTags:    []string{"alskdjfalskdjfalksdjfklasjdfljasdkfj"},
			RepoDigests: []string{"alskdjfalskdjfalksdjfklasjdfljasdkfj"},
			Size_:       uint64(2000),
			Username:    "",
		},
	}

	var err error
	c.wasmModuleStore, err = wasmstore.NewFakeStore(wasmsInStore)
	assert.NoError(t, err)

	resp, err := c.ListImages(context.Background(), &runtime.ListImagesRequest{})
	assert.NoError(t, err)
	require.NotNil(t, resp)
	images := resp.GetImages()
	assert.Len(t, images, len(expect))
	for _, i := range expect {
		assert.Contains(t, images, i)
	}
}

func TestListImages(t *testing.T) {
	c := newTestCRIService()
	imagesInStore := []imagestore.Image{
		{
			ID:      "sha256:1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ChainID: "test-chainid-1",
			References: []string{
				"gcr.io/library/busybox:latest",
				"gcr.io/library/busybox@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582",
			},
			Size: 1000,
			ImageSpec: imagespec.Image{
				Config: imagespec.ImageConfig{
					User: "root",
				},
			},
		},
		{
			ID:      "sha256:2123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ChainID: "test-chainid-2",
			References: []string{
				"gcr.io/library/alpine:latest",
				"gcr.io/library/alpine@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582",
			},
			Size: 2000,
			ImageSpec: imagespec.Image{
				Config: imagespec.ImageConfig{
					User: "1234:1234",
				},
			},
		},
		{
			ID:      "sha256:3123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ChainID: "test-chainid-3",
			References: []string{
				"gcr.io/library/ubuntu:latest",
				"gcr.io/library/ubuntu@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582",
			},
			Size: 3000,
			ImageSpec: imagespec.Image{
				Config: imagespec.ImageConfig{
					User: "nobody",
				},
			},
		},
	}
	expect := []*runtime.Image{
		{
			Id:          "sha256:1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			RepoTags:    []string{"gcr.io/library/busybox:latest"},
			RepoDigests: []string{"gcr.io/library/busybox@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582"},
			Size_:       uint64(1000),
			Username:    "root",
		},
		{
			Id:          "sha256:2123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			RepoTags:    []string{"gcr.io/library/alpine:latest"},
			RepoDigests: []string{"gcr.io/library/alpine@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582"},
			Size_:       uint64(2000),
			Uid:         &runtime.Int64Value{Value: 1234},
		},
		{
			Id:          "sha256:3123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			RepoTags:    []string{"gcr.io/library/ubuntu:latest"},
			RepoDigests: []string{"gcr.io/library/ubuntu@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582"},
			Size_:       uint64(3000),
			Username:    "nobody",
		},
	}

	var err error
	c.imageStore, err = imagestore.NewFakeStore(imagesInStore)
	assert.NoError(t, err)

	resp, err := c.ListImages(context.Background(), &runtime.ListImagesRequest{})
	assert.NoError(t, err)
	require.NotNil(t, resp)
	images := resp.GetImages()
	assert.Len(t, images, len(expect))
	for _, i := range expect {
		assert.Contains(t, images, i)
	}
}
