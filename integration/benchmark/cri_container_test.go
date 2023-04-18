package benchmark

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"testing"
	"time"
)

func BenchmarkWasmInstanceInCri(b *testing.B) {
	b.Logf("Create a pod config and run wasm instance")
	sb, sbConfig := PodSandboxConfigWithCleanup(b, "sandbox1", "wasm",
		WithPodLogDirectory("/tmp"))

	wasmModule := &runtime.ImageSpec{
		Image: "wasm-example",
		Annotations: map[string]string{
			"wasm.module.url":      "https://github.com/leviyanx/wasm-program-image/raw/main/wasi/wasi_example_main.wasm",
			"wasm.module.filename": "wasi_example_main.wasm",
		},
	}

	EnsureWasmModuleExists(b, *wasmModule)

	b.Logf("Create a wasm instance in a pod")
	containerConfig := ContainerConfigWithWasmModule(
		"container1",
		wasmModule,
		WithTestLabels(),
		WithTestAnnotations(),
		WithLogPath("container1.log"),
		WithCommand("wasi_example_main.wasm", "test"),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(b, err)
	defer func() {
		b.Logf("Remove the wasm instance")
		assert.NoError(b, runtimeService.RemoveContainer(cn))
	}()

	b.Logf("Start the wasm instance in the pod")
	require.NoError(b, runtimeService.StartContainer(cn))
	time.Sleep(time.Second)
	defer func() {
		b.Logf("Stop the wasm instance")
		assert.NoError(b, runtimeService.StopContainer(cn, 1))
	}()
}

func BenchmarkContainerInCri(b *testing.B) {
	b.Logf("Create a pod config and run sandbox container")
	sb, sbConfig := PodSandboxConfigWithCleanup(b, "sandbox2", "container",
		WithPodLogDirectory("/tmp"))

	EnsureImageExists(b, pauseImage)

	b.Logf("Create a container config and run container in a pod")
	containerConfig := ContainerConfig(
		"container2",
		pauseImage,
		WithTestLabels(),
		WithTestAnnotations(),
		WithLogPath("container2.log"),
	)
	cn, err := runtimeService.CreateContainer(sb, containerConfig, sbConfig)
	require.NoError(b, err)
	defer func() {
		b.Logf("Remove the container")
		assert.NoError(b, runtimeService.RemoveContainer(cn))
	}()
	require.NoError(b, runtimeService.StartContainer(cn))
	time.Sleep(time.Second)
	defer func() {
		b.Logf("Stop the container")
		assert.NoError(b, runtimeService.StopContainer(cn, 1))
	}()
}
