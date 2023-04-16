package wasminstance

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/containerd/containerd"
	wasmdealer "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/api/types/task"
	containerdio "github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	cio "github.com/containerd/containerd/pkg/cri/io"
	"github.com/containerd/containerd/pkg/cri/store"
	"github.com/containerd/containerd/pkg/cri/store/label"
	"github.com/containerd/containerd/pkg/cri/store/truncindex"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	"github.com/containerd/fifo"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type IOCreator func(id string) (containerdio.IO, error)

type WasmInterface interface {
	ID() string

	GetWasmModule() wasmmodule.WasmModule

	// NewTask creates a new task based on the wasm instance
	NewTask(ctx context.Context, client *containerd.Client, ioCreator IOCreator) (WasmTask, error)

	// Task returns the current task for the wasm instance
	//
	// If containerio.Attach options are passed the client will reattch to the IO for the running
	// task. If no task exists for the wasm instance, a NotFound error is returned.
	//
	// Clients must make sure that only one reader is attached to the task and consuming
	// the output from the task's fifos
	Task(context.Context, containerdio.Attach) (WasmTask, error)
}

// WasmInstance contains all resources associated with the wasm instance.
type WasmInstance struct {
	// Metadata is the metadata of the wasm instance, it is immutable after created.
	Metadata

	// client is the containerd client.
	client *containerd.Client

	// Status stores the status of the wasm instance.
	Status StatusStorage

	// Wasm instance IO.
	// IO could only be nil when the wasm instance is in unknown state.
	IO *cio.ContainerIO

	// StopCh is used to propagate the stop information of the wasm instance.
	*store.StopCh

	// IsStopSignaledWithTimeout the default is 0, and it is set to 1 after sending
	// the signal once to avoid repeated sending of the signal.
	IsStopSignaledWithTimeout *uint32

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
			anyType *anypb.Any
			err     error
		)
		if options != nil {
			var typesAny *types.Any // typesAny is used as a bridge to convert options to anypb.Any
			typesAny, err = typeurl.MarshalAny(options)
			if err != nil {
				return err
			}
			anyType = &anypb.Any{
				TypeUrl: typesAny.TypeUrl,
				Value:   typesAny.Value,
			}
		}
		w.Runtime = RuntimeInfo{
			Name:    name,
			Options: anyType,
		}
		return nil
	}
}
func WithSpec(s *oci.Spec) NewWasmInstanceOpts {
	return func(ctx context.Context, w *WasmInstance) error {
		typesAny, err := typeurl.MarshalAny(s)
		if err != nil {
			return err
		}
		w.Spec = &anypb.Any{
			TypeUrl: typesAny.TypeUrl,
			Value:   typesAny.Value,
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

// WithWasmInstanceIO adds IO into the wasm instance.
func WithWasmInstanceIO(io *cio.ContainerIO) NewWasmInstanceOpts {
	return func(ctx context.Context, w *WasmInstance) error {
		w.IO = io
		return nil
	}
}

func NewWasmInstance(ctx context.Context, metadata Metadata, client *containerd.Client, opts ...NewWasmInstanceOpts) (WasmInstance, error) {
	wasmInstance := WasmInstance{
		Metadata:                  metadata,
		client:                    client,
		StopCh:                    store.NewStopCh(),
		IsStopSignaledWithTimeout: new(uint32),
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
	return w.Status.Delete()
}

func (w *WasmInstance) ID() string {
	return w.Metadata.ID
}

func (w *WasmInstance) GetWasmModule() wasmmodule.WasmModule {
	return w.WasmModule
}

func (w *WasmInstance) NewTask(ctx context.Context, client *containerd.Client, ioCreator IOCreator) (WasmTask, error) {
	io, err := ioCreator(w.ID())
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil && io != nil {
			io.Cancel()
			io.Close()
		}
	}()

	cfg := io.Config()
	request := &wasmdealer.CreateTaskRequest{
		WasmId:         w.ID(),
		ImagePath:      w.WasmModule.GetFilepath(),
		Spec:           w.Spec,
		Stdin:          cfg.Stdin,
		Stdout:         cfg.Stdout,
		Stderr:         cfg.Stderr,
		RuntimeOptions: w.Runtime.Options,
		Runtime:        w.Runtime.Name,
	}

	wasmTask := &wasmTask{
		client: client,
		io:     io,
		id:     w.ID(),
	}

	responce, err := client.WasmdealerService().Create(ctx, request)
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}
	wasmTask.pid = responce.GetPid()
	return wasmTask, nil
}

func (w *WasmInstance) Task(ctx context.Context, ioAttach containerdio.Attach) (WasmTask, error) {
	response, err := w.client.WasmdealerService().Get(ctx, &wasmdealer.GetRequest{
		WasmId: w.ID(),
	})
	if err != nil {
		err = errdefs.FromGRPC(err)
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("no running wasm task found: %w", err)
		}
		return nil, err
	}

	var io containerdio.IO
	if ioAttach != nil && response.GetProcess().Status != task.StatusUnknown {
		// Do not attach IO for task in unknown state, because there are
		// no fifo paths anyway.
		if io, err = attachExistingIO(response, ioAttach); err != nil {
			return nil, err
		}
	}
	wasmTask := &wasmTask{
		client: w.client,
		io:     io,
		id:     response.GetProcess().ID,
		pid:    response.GetProcess().Pid,
	}
	return wasmTask, nil
}

// get the existing fifio paths from the task information store by the daemon
func attachExistingIO(response *wasmdealer.GetResponse, ioAttach containerdio.Attach) (containerdio.IO, error) {
	fifoSet := loadFifos(response)
	return ioAttach(fifoSet)
}

// loadFifos loads the wasm instance fifos
func loadFifos(response *wasmdealer.GetResponse) *containerdio.FIFOSet {
	fifos := []string{
		response.Process.Stdin,
		response.Process.Stdout,
		response.Process.Stderr,
	}
	closer := func() error {
		var (
			err  error
			dirs = map[string]struct{}{}
		)
		for _, f := range fifos {
			if isFifo, _ := fifo.IsFifo(f); isFifo {
				if rerr := os.Remove(f); err == nil {
					err = rerr
				}
				dirs[filepath.Dir(f)] = struct{}{}
			}
		}
		for dir := range dirs {
			// we ignore errors here because we don't
			// want to remove the directory if it isn't
			// empty
			os.Remove(dir)
		}
		return err
	}

	return containerdio.NewFIFOSet(containerdio.Config{
		Stdin:    response.Process.Stdin,
		Stdout:   response.Process.Stdout,
		Stderr:   response.Process.Stderr,
		Terminal: response.Process.Terminal,
	}, closer)
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

// List lists all wasm instances.
func (s *Store) List() []WasmInstance {
	s.lock.RLock()
	defer s.lock.RUnlock()
	var instances []WasmInstance
	for _, c := range s.wasmInstances {
		instances = append(instances, c)
	}
	return instances
}
