package benchmark

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd/errdefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"testing"
	"time"
)

func BenchmarkWasmModuleInCri(b *testing.B) {
	testWasmModuleName := "wasi_example_main" // This is the name of the wasm module
	image := &runtime.ImageSpec{
		Image: testWasmModuleName,
		Annotations: map[string]string{
			"wasm.module.url":       "https://github.com/leviyanx/wasm-program-image/raw/main/wasi/wasi_example_main.wasm",
			"wasm.module.extension": "*.wasm",
		},
	}

	b.Logf("make sure the test wasm moduel doesn't exist in the cri plugin")
	i, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testWasmModuleName})
	require.NoError(b, err)
	if i != nil {
		b.Logf("remove the wasm module from the cri plugin")
		require.NoError(b, imageService.RemoveImage(&runtime.ImageSpec{Image: testWasmModuleName}))
	}

	b.Logf("pull the wasm module into the cri plugin")
	_, err = imageService.PullImage(image, nil, nil)
	assert.NoError(b, err)
	defer func() {
		b.Logf("remove the wasm module from the cri plugin")
		assert.NoError(b, imageService.RemoveImage(&runtime.ImageSpec{Image: testWasmModuleName}))
	}()

	b.Logf("the wasm module should be seen in the cri plugin")
	//var id string
	checkWasmModule := func() (bool, error) {
		w, err := imageService.ImageStatus(image)
		if err != nil {
			return false, err
		}
		if w == nil {
			b.Logf("Wasm module %q not show up in the cri plugin yet", testWasmModuleName)
			return false, nil
		}

		// TODO: support referred by id
		//id = w.Id
		//w, err = imageService.ImageStatus(&runtime.ImageSpec{Image: id})
		//if err != nil {
		//	return false, err
		//}
		//if w == nil {
		//	// We always generate image id as a reference first, it must
		//	// be ready here.
		//	return false, errors.New("can't reference wasm module by id")
		//}

		// TODO: support RepoTags
		//if len(w.RepoTags) != 1 {
		//	// RepoTags must have been populated correctly.
		//	return false, fmt.Errorf("unexpected repotags: %+v", w.RepoTags)
		//}
		//if w.RepoTags[0] != testWasmModuleName {
		//	return false, fmt.Errorf("unexpected repotag %q", w.RepoTags[0])
		//}
		return true, nil
	}

	require.NoError(b, Eventually(checkWasmModule, 100*time.Millisecond, 10*time.Second))
	require.NoError(b, Consistently(checkWasmModule, 100*time.Millisecond, time.Second))
}

func BenchmarkImageInCri(b *testing.B) {
	var testImage = GetImage(BusyBox)
	ctx := context.Background()

	b.Logf("make sure the test image doesn't exist in the cri plugin")
	i, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testImage})
	require.NoError(b, err)
	if i != nil {
		b.Logf("remove the image from the cri plugin")
		require.NoError(b, imageService.RemoveImage(&runtime.ImageSpec{Image: testImage}))
	}

	b.Logf("pull the image into the cri plugin")
	_, err = imageService.PullImage(&runtime.ImageSpec{Image: testImage}, nil, nil)
	assert.NoError(b, err)
	defer func() {
		// Make sure the image is cleaned up in any case.
		if err := containerdClient.ImageService().Delete(ctx, testImage); err != nil {
			assert.True(b, errdefs.IsNotFound(err), err)
		}
		assert.NoError(b, imageService.RemoveImage(&runtime.ImageSpec{Image: testImage}))
	}()

	b.Logf("the image should be seen in the cri plugin")
	var id string
	checkImage := func() (bool, error) {
		img, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testImage})
		if err != nil {
			return false, err
		}
		if img == nil {
			b.Logf("Image %q not show up in the cri plugin yet", testImage)
			return false, nil
		}
		id = img.Id
		img, err = imageService.ImageStatus(&runtime.ImageSpec{Image: id})
		if err != nil {
			return false, err
		}
		if img == nil {
			// We always generate image id as a reference first, it must
			// be ready here.
			return false, errors.New("can't reference image by id")
		}
		if len(img.RepoTags) != 1 {
			// RepoTags must have been populated correctly.
			return false, fmt.Errorf("unexpected repotags: %+v", img.RepoTags)
		}
		if img.RepoTags[0] != testImage {
			return false, fmt.Errorf("unexpected repotag %q", img.RepoTags[0])
		}
		return true, nil
	}
	require.NoError(b, Eventually(checkImage, 100*time.Millisecond, 10*time.Second))
	require.NoError(b, Consistently(checkImage, 100*time.Millisecond, time.Second))
}
