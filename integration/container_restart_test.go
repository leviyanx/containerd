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
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test to verify wasm instance can be restarted
func TestWasmInstanceRestart(t *testing.T) {
	t.Logf("Create a pod config and run wasm instance")
	sb, sbConfig := PodSandboxConfigWithCleanup(t, "sandbox1", "restart",
		WithPodLogDirectory("/tmp"))

	wasmModule := &runtime.ImageSpec{
		Image: "wasm-example",
		Annotations: map[string]string{
			"wasm.module.url":      "https://github.com/leviyanx/wasm-program-image/raw/main/wasi/wasi_example_main.wasm",
			"wasm.module.filename": "wasi_example_main.wasm",
		},
	}

	EnsureWasmModuleExists(t, *wasmModule)

	t.Logf("Create a wasm instance in a pod")
	containerConfig := ContainerConfigWithWasmModule(
		"container1",
		wasmModule,
		WithTestLabels(),
		WithTestAnnotations(),
		WithLogPath("container1.log"),
		WithCommand("wasi_example_main.wasm", "test"),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	defer func() {
		t.Logf("Remove the wasm instance")
		assert.NoError(t, runtimeService.RemoveContainer(cn))
	}()

	t.Logf("Start the wasm instance in the pod")
	require.NoError(t, runtimeService.StartContainer(cn))
	time.Sleep(time.Second)
	defer func() {
		t.Logf("Stop the wasm instance")
		assert.NoError(t, runtimeService.StopContainer(cn, 10))
	}()

	t.Logf("Restart the wasm instance with same config")
	require.NoError(t, runtimeService.StopContainer(cn, 10))
	require.NoError(t, runtimeService.RemoveContainer(cn))

	cn, err = runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	require.NoError(t, runtimeService.StartContainer(cn))
	time.Sleep(time.Second)
}

// Test to verify container can be restarted
func TestContainerRestart(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sb, sbConfig := PodSandboxConfigWithCleanup(t, "sandbox1", "restart")

	EnsureImageExists(t, pauseImage)

	t.Logf("Create a container config and run container in a pod")
	containerConfig := ContainerConfig(
		"container1",
		pauseImage,
		WithTestLabels(),
		WithTestAnnotations(),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.RemoveContainer(cn))
	}()
	require.NoError(t, runtimeService.StartContainer(cn))
	defer func() {
		assert.NoError(t, runtimeService.StopContainer(cn, 10))
	}()

	t.Logf("Restart the container with same config")
	require.NoError(t, runtimeService.StopContainer(cn, 10))
	require.NoError(t, runtimeService.RemoveContainer(cn))

	cn, err = runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	require.NoError(t, runtimeService.StartContainer(cn))
}

// Test to verify that, after a container fails to start due to a bad command, it can be removed
// and a proper container can be created and started in its stead.
func TestFailedContainerRestart(t *testing.T) {
	t.Logf("Create a pod config and run sandbox container")
	sb, sbConfig := PodSandboxConfigWithCleanup(t, "sandbox1", "restart")

	EnsureImageExists(t, pauseImage)

	t.Logf("Create a container config in a pod with a command that fails")
	containerConfig := ContainerConfig(
		"container1",
		pauseImage,
		WithCommand("something-that-doesnt-exist"),
		WithTestLabels(),
		WithTestAnnotations(),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, runtimeService.RemoveContainer(cn))
	}()
	require.Error(t, runtimeService.StartContainer(cn))
	defer func() {
		assert.NoError(t, runtimeService.StopContainer(cn, 10))
	}()

	t.Logf("Create the container with a proper command")
	require.NoError(t, runtimeService.StopContainer(cn, 10))
	require.NoError(t, runtimeService.RemoveContainer(cn))

	containerConfig = ContainerConfig(
		"container1",
		pauseImage,
		WithTestLabels(),
		WithTestAnnotations(),
	)
	cn, err = runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(t, err)
	require.NoError(t, runtimeService.StartContainer(cn))
}
