package containerd

import (
	"github.com/containerd/containerd/wasmmodules"
)

// WasmModule describe a wasm module used by wasm runtime
type WasmModule interface {
	// Name of the wasm module
	Name() string

	// Target descriptor for the wasm module content
	Target() wasmmodules.WasmModuleSpec

	// Size return the size of the wasm module
	Size() (int64, error)

	// Metadata returns the underlying wasm module metadata
	Metadata() wasmmodules.WasmModule
}

type wasmModule struct {
	client *Client

	wasmModule wasmmodules.WasmModule
}

func (w *wasmModule) Metadata() wasmmodules.WasmModule {
	return w.wasmModule
}

func (w *wasmModule) Name() string {
	return w.wasmModule.Name
}

func (w *wasmModule) Target() wasmmodules.WasmModuleSpec {
	return w.wasmModule.Target
}

func (w *wasmModule) Size() (int64, error) {
	return w.wasmModule.Target.Size, nil
}
