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

package integration

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Test to test the wasm module operations in CRI plugin.
func TestWasmModuleInCri(t *testing.T) {
	testWasmModuleName = "wasi_example_main" // This is the name of the wasm module
	testWasmModuleAnnotation := map[string]string{
		"wasm.module.url": "https://github.com/leviyanx/wasm-program-image/raw/main/wasi/wasi_example_main.wasm",
	}
	ctx := context.Background()

	t.Logf("make sure the test wasm moduel doesn't exist in the cri plugin")
	i, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testWasmModuleName})
	require.NoError(t, err)
	if i != nil {
		require.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: testImage}))
	}

	t.Logf("pull the wasm module into the cri plugin")
	_, err = imageService.PullImage(&runtime.ImageSpec{Image: testWasmModuleName, Annotations: testWasmModuleAnnotation}, nil, nil)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, imageService.RemoveImage(&runtime.ImageSpec{Image: testWasmModuleName}))
	}()

	t.Logf("the wasm module should be seen in the cri plugin")
	var id string
	checkWasmModule := func() (bool, error) {
		w, err := imageService.ImageStatus(&runtime.ImageSpec{Image: testWasmModuleName})
		if err != nil {
			return false, err
		}
		if w == nil {
			t.Logf("Wasm module %q not show up in the cri plugin yet", testWasmModuleName)
			return false, nil
		}
		id = w.Id
		w, err = imageService.ImageStatus(&runtime.ImageSpec{Image: id})
		if err != nil {
			return false, err
		}
		if w == nil {
			// We always generate image id as a reference first, it must
			// be ready here.
			return false, errors.New("can't reference wasm module by id")
		}
		if len(w.RepoTags) != 1 {
			// RepoTags must have been populated correctly.
			return false, fmt.Errorf("unexpected repotags: %+v", w.RepoTags)
		}
		if w.RepoTags[0] != testWasmModuleName {
			return false, fmt.Errorf("unexpected repotag %q", w.RepoTags[0])
		}
		return true, nil
	}

	require.NoError(t, Eventually(checkWasmModule, 100*time.Millisecond, 10*time.Second))
	require.NoError(t, Consistently(checkWasmModule, 100*time.Millisecond, time.Second))
}
