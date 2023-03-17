package wasmmodule

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/wasmmodules"
	"sync"
	"time"
)

// WasmModule (cache) contains all resources associated with the wasm module. All fields Must not be mutated
// directly after created.
type WasmModule struct {
	// ID of the wasm module. Normally the digest of wasm module
	ID string

	// Name of the wasm module.
	// Required
	Name string

	// ChainID is the chainID of the wasm module
	ChainID string

	Size int64

	// WasmModuleSpec describes basic information about the wasm module
	WasmModuleSpec wasmmodules.WasmModuleSpec

	CreatedAt, UpdatedAt time.Time
}

// Store stores all wasm modules
type Store struct {
	lock sync.RWMutex

	// client is the containerd client
	client *containerd.Client

	// store is the internal wasm module store index indexed by wasm module id
	store *store
}

type store struct {
	lock sync.RWMutex
	// map: id - wasm module
	wasmModules map[string]WasmModule
	// wasm module id set (id - image)
	idSet map[string]string
}
