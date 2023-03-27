package wasminstance

import runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

type Metadata struct {
	// ID is the wasm instance id.
	ID string
	// Name is the wasm instance name.
	Name string
	// WasmModuleID is the wasm module id the wasm instance belongs to.
	SandboxID string
	// Config is the CRI container config.
	Config *runtime.ContainerConfig
	// WasmModuleName is the name of the wasm module used by the wasm instance.
	WasmModuleName string
	// LogPath is the wasm instance log path.
	StopSignal string
}
