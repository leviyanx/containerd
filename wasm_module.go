package containerd

import (
	"context"
	"github.com/containerd/containerd/wasmmodules"
)

// WasmModule describe a wasm module used by wasm runtime
type WasmModule interface {
	// Name of the wasm module
	Name() string

	// Target descriptor for the wasm module content
	Target() wasmmodules.WasmModuleSpec

	// Size return the size of the wasm module
	Size(ctx context.Context) (int64, error)

	// Metadata returns the underlying wasm module metadate
	Metadata() wasmmodules.WasmModule
}
