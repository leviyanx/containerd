package wasminstance

import (
	"github.com/containerd/containerd/pkg/cri/store/label"
	"github.com/containerd/containerd/pkg/cri/store/truncindex"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"golang.org/x/net/context"
	"sync"
)

type WasmInterface interface {
	ID() string

	WasmModule(ctx context.Context) (wasmmodule.WasmModule, error)
}

// WasmInstance contains all resources associated with the wasm instance.
type WasmInstance struct {
	// Metadata is the metadata of the wasm instance, it is immutable after created.
	Metadata

	// WasmModule is the wasm module the wasm instance belongs to.
	WasmModule wasmmodule.WasmModule
}

// Store stores all wasm instances
type Store struct {
	lock          sync.RWMutex
	wasmInstances map[string]WasmInstance
	idIndex       *truncindex.TruncIndex
	labels        *label.Store
}
