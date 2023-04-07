package wasminstance

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/pkg/cri/store"
	"github.com/containerd/containerd/pkg/cri/store/label"
	"github.com/containerd/containerd/pkg/cri/store/truncindex"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
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

	// Status stores the status of the wasm instance.
	Status StatusStorage

	// StopCh is used to propagate the stop information of the wasm instance.
	*store.StopCh

	// WasmModule is the wasm module the wasm instance belongs to.
	WasmModule wasmmodule.WasmModule
}

// NewWasmInstanceOpts sets specific information to newly created WasmInstance.
type NewWasmInstanceOpts func(ctx context.Context, w *WasmInstance) error

// WithRuntime allows a user to specify the runtime name and additional options
// that should be used to create tasks for the wasm instance.
func WithRuntime(name string, options interface{}) NewWasmInstanceOpts {
	return func(ctx context.Context, w *WasmInstance) error {
		var (
			anyType *types.Any
			err     error
		)
		if options != nil {
			anyType, err = typeurl.MarshalAny(options)
			if err != nil {
				return err
			}
		}
		w.Runtime = containers.RuntimeInfo{
			Name:    name,
			Options: anyType,
		}
		return nil
	}
}

func WithStatus(status Status, root string) NewWasmInstanceOpts {
	return func(ctx context.Context, w *WasmInstance) error {
		s, err := StoreStatus(root, w.ID(), status)
		if err != nil {
			return err
		}
		w.Status = s
		if s.Get().State() == runtime.ContainerState_CONTAINER_EXITED {
			w.Stop()
		}
		return nil
	}

}

// WithWasmModule sets the provided wasm module as the base for the wasm instance.
func WithWasmModule(wasmModule wasmmodule.WasmModule) NewWasmInstanceOpts {
	return func(ctx context.Context, w *WasmInstance) error {
		w.WasmModule = wasmModule
		return nil
	}
}

func NewWasmInstance(ctx context.Context, metadata Metadata, opts ...NewWasmInstanceOpts) (WasmInstance, error) {
	wasmInstance := WasmInstance{
		Metadata: metadata,
		StopCh:   store.NewStopCh(),
	}

	for _, o := range opts {
		if err := o(ctx, &wasmInstance); err != nil {
			return WasmInstance{}, err
		}
	}

	return wasmInstance, nil
}

// Delete deletes checkpoint for the wasm instance
func (w *WasmInstance) Delete() error {
	// TODO: call wasmInstance.status.Delete()
	return nil
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

// Add a wasm instance to the store. Return store.ErrAlreadyExists if the
// wasm instance already exists.
func (s *Store) Add(w WasmInstance) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// check if wasm instance already exists
	if _, ok := s.wasmInstances[w.ID()]; ok {
		return errdefs.ErrAlreadyExists
	}

	if err := s.idIndex.Add(w.ID()); err != nil {
		return err
	}
	s.wasmInstances[w.ID()] = w
	return nil
}

// Get returns the wasm instance with specified ID. Return store.ErrNotExist if
// the wasm instance doesn't exist.
func (s *Store) Get(id string) (WasmInstance, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	id, err := s.idIndex.Get(id)
	if err != nil {
		if err == truncindex.ErrNotExist {
			err = errdefs.ErrNotFound
		}
		return WasmInstance{}, err
	}

	if c, ok := s.wasmInstances[id]; ok {
		return c, nil
	}
	return WasmInstance{}, errdefs.ErrNotFound
}
