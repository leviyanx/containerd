package wasmdealer

import (
	"context"
	"fmt"

	api "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/services"
	"google.golang.org/grpc"
)

func init() {
	plugin.Register(&plugin.Registration{
		Type:     plugin.ServicePlugin,
		ID:       services.WasmdealerService,
		Requires: []plugin.Type{
			plugin.RuntimePluginV2,
		},
		InitFn:   initLocal,
	})
}

func initLocal(ic *plugin.InitContext) (interface{}, error) {
  v2r, err := ic.GetByID(plugin.RuntimePluginV2, "task")
	if err != nil {
		return nil, err
	}

	monitor, err := ic.Get(plugin.TaskMonitorPlugin)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return nil, err
		}
		monitor = runtime.NewNoopMonitor()
	}

  l := &local {
		monitor:    monitor.(runtime.TaskMonitor),
    runtime: v2r.(runtime.PlatformRuntime),
  }

	tasks, err := l.runtime.Tasks(ic.Context, true)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		l.monitor.Monitor(t, nil)
	}

  // TODO: what does RDT init do in task runtime plugin init, do i need to init it again
	// if err := initRdt(config.RdtConfigFile); err != nil {
	// 	log.G(ic.Context).WithError(err).Errorf("RDT initialization failed")
	// }

  return l, nil

}

type local struct {
	monitor   runtime.TaskMonitor
  runtime runtime.PlatformRuntime
}

// TODO: add a test case in test_plugin, seemds hard :(
func (l *local) Create(ctx context.Context, r *api.CreateTaskRequest, _ ...grpc.CallOption) (*api.CreateTaskResponse, error) {
	opts := runtime.CreateOpts{
		Spec: anyFromPbToTypes(r.Spec),
		IO: runtime.IO{
			Stdin:    r.Stdin,
			Stdout:   r.Stdout,
			Stderr:   r.Stderr,
			Terminal: false,
		},
		Runtime:        r.Runtime,
    RuntimeOptions: anyFromPbToTypes(r.RuntimeOptions),
		TaskOptions:    anyFromPbToTypes(r.TaskOptions),
	}

  // TODO: mount wasm path to main rootfas

  _, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil && err != runtime.ErrTaskNotExists {
		return nil, errdefs.ToGRPC(err)
	}
	if err == nil {
		return nil, errdefs.ToGRPC(fmt.Errorf("task %s: %w", r.WasmId, errdefs.ErrAlreadyExists))
	}
	c, err := l.runtime.Create(ctx, r.WasmId, opts)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	labels := map[string]string{"runtime": r.Runtime}
	if err := l.monitor.Monitor(c, labels); err != nil {
		return nil, fmt.Errorf("monitor task: %w", err)
	}

	pid, err := c.PID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get task pid: %w", err)
	}
	return &api.CreateTaskResponse{
		WasmId: r.WasmId,
		Pid:         pid,
	}, nil
}

func (l *local) Start(ctx context.Context, r *api.StartRequest, _ ...grpc.CallOption) (*api.StartResponse, error) {
  return nil, nil
}

func (l *local) Delete(ctx context.Context, r *api.DeleteTaskRequest, _ ...grpc.CallOption) (*api.DeleteResponse, error) {
  return nil, nil
}

func (l *local) DeleteProcess(ctx context.Context, r *api.DeleteProcessRequest, _ ...grpc.CallOption) (*api.DeleteResponse, error) {
  return nil, nil
}

func (l *local) Get(ctx context.Context, r *api.GetRequest, _ ...grpc.CallOption) (*api.GetResponse, error) {
  return nil, nil

}

func (l *local) List(ctx context.Context, r *api.ListTasksRequest, _ ...grpc.CallOption) (*api.ListTasksResponse, error) {
  return nil, nil
}

