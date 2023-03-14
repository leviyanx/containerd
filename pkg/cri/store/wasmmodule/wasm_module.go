package wasmmodule

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/wasmmodules"
	"sync"
)

// WasmModule (cache) contains all resources associated with the wasm module. All fields Must not be mutated
// directly after created.
type WasmModule struct {
	// ID of the wasm module.
	ID string

	// ChainID is the chainID of the wasm module
	ChainID string

	Size int64

	// WasmModuleSpec describes basic information about the wasm module
	WasmModuleSpec wasmmodules.WasmModuleSpec
}

// Store stores all wasm modules
type Store struct {
	lock sync.RWMutex

	refCache map[string]string

	// client is the containerd client
	client *containerd.Client

	// store is the internal wasm module store index indexed by wasm module id
	store *store
}

type store struct {
	lock        sync.RWMutex
	wasmModules map[string]WasmModule
	// ID set
	idSet
}
