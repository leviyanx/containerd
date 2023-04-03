package wasminstance

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/pkg/cri/store/label"
	"github.com/containerd/containerd/pkg/cri/store/truncindex"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"golang.org/x/net/context"
	"sync"
)

type WasmInterface interface {
	ID() string

	GetWasmModule(ctx context.Context) (wasmmodule.WasmModule, error)

	Task(ctx context.Context, attach cio.Attach) (containerd.Task, error)
}

// WasmInstance contains all resources associated with the wasm instance.
type WasmInstance struct {
	// Metadata is the metadata of the wasm instance, it is immutable after created.
	Metadata

	// WasmModule is the wasm module the wasm instance belongs to.
	WasmModule wasmmodule.WasmModule
}

// Opts sets specific information to newly created WasmInstance.
type Opts func(*WasmInstance) error

func NewWasmInstance(metadata Metadata, opts ...Opts) (WasmInstance, error) {
	wasmInstance := WasmInstance{
		Metadata: metadata,
	}

	for _, o := range opts {
		if err := o(&wasmInstance); err != nil {
			return WasmInstance{}, err
		}
	}

	return wasmInstance, nil
}

func (w *WasmInstance) ID() string {
	return w.Metadata.ID
}

func (w *WasmInstance) GetWasmModule(ctx context.Context) (wasmmodule.WasmModule, error) {
	return w.WasmModule, nil
}

// Store stores all wasm instances
type Store struct {
	lock          sync.RWMutex
	wasmInstances map[string]WasmInstance
	idIndex       *truncindex.TruncIndex
	labels        *label.Store
}

// NewStore creates a wasm instance store.
func NewStore(labels *label.Store) *Store {
	return &Store{
		wasmInstances: make(map[string]WasmInstance),
		idIndex:       truncindex.NewTruncIndex([]string{}),
		labels:        labels,
	}
}
