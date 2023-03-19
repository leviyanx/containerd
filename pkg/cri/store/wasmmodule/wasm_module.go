package wasmmodule

import (
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
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

func IsWasmModule(image v1.ImageSpec) bool {
	// if the annotation has the wasm module url item, it is a wasm module
	_, urlExists := image.GetAnnotations()["wasm.module.url"]
	return urlExists
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

// Resolve resolves the name to the corresponding wasm module id.
func (s *Store) Resolve(name string) (id string, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	id, ok := s.nameSet[name]
	if !ok {
		return "", fmt.Errorf("wasm module %q not found", name)
	}
	return id, nil
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

func (s *Store) Delete(name string) error {
	id, err := s.Resolve(name)
	if err != nil {
		return err
	}

	// delete the wasm module from the store
	err = s.store.delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete wasm module %q: %v", id, err)
	}

	// update the name set
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.nameSet, name)

	return nil
}

func (s *Store) Get(name string) (WasmModule, error) {
	id, err := s.Resolve(name)
	if err != nil {
		return WasmModule{}, errdefs.ErrNotFound
	}

	return s.store.get(id)
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

func (s *store) get(id string) (WasmModule, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	wasmModule, ok := s.wasmModules[id]
	if !ok {
		return WasmModule{}, errdefs.ErrNotFound
	}
	return wasmModule, nil
}

func (s *store) delete(id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.wasmModules[id]; !ok {
		return fmt.Errorf("wasm module %q not found in store", id)
	}

	delete(s.wasmModules, id)
	return nil
}
