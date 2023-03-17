package wasmmodule

import (
	"fmt"
	"github.com/containerd/containerd"
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

	// Filepath is the path that store wasm module file in local.
	Filepath string

	Size int64

	// WasmModuleSpec describes basic information about the wasm module
	WasmModuleSpec WasmModuleSpec

	CreatedAt, UpdatedAt time.Time
}

type WasmModuleSpec struct {
	// Basic info >>>
	Author string
	// <<< Basic info

	// Descriptor >>>
	// Size is the size of the wasm module
	Size int64

	// Name of the wasm runtime running the module
	Runtime string

	// URL specifies the URL from which this wasm module MAY be downloaded
	URL string

	// Annotations contains arbitrary metadata relating to the targeted wasm module.
	Annotations map[string]string
	// <<< Descriptor

	// Config >>>
	// Cmd defines the default command for the wasm module / wasm instance
	Cmd []string
	// <<< Config
}

// Store stores all wasm modules
type Store struct {
	lock sync.RWMutex

	// wasm module name set (name - id)
	nameSet map[string]string

	// client is the containerd client
	client *containerd.Client

	// store is the internal wasm module store index indexed by wasm module id
	store *store
}

func NewStore(client *containerd.Client) *Store {
	return &Store{
		client:  client,
		nameSet: make(map[string]string),
		store: &store{
			wasmModules: make(map[string]WasmModule),
		},
	}
}

// Add adds a new wasm module to the store.
func (s *Store) Add(wasmModule WasmModule) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.nameSet[wasmModule.Name]; ok {
		return fmt.Errorf("wasm module %q already exists", wasmModule.Name)
	}

	// store the wasm module into the store
	err := s.store.add(&wasmModule)
	if err != nil {
		return fmt.Errorf("failed to add wasm module %q: %v", wasmModule.Name, err)
	}

	// update the name set
	s.nameSet[wasmModule.Name] = wasmModule.ID
	return nil
}

type store struct {
	lock sync.RWMutex
	// map: id - wasm module
	wasmModules map[string]WasmModule
}

func (s *store) add(wasmModule *WasmModule) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.wasmModules[wasmModule.ID]; ok {
		return fmt.Errorf("wasm module %q already exists", wasmModule.ID)
	}

	s.wasmModules[wasmModule.ID] = *wasmModule
	return nil
}
