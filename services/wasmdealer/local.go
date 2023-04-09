package wasmdealer

import (
	"context"
	"fmt"

	api "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/timeout"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/services"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	_     = (api.WasmdealerClient)(&local{})
	empty = &types.Empty{}
)

const (
	stateTimeout = "io.containerd.timeout.task.state"
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

// TODO: add test cases for apis in test_plugin, seemds hard :(
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
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task %v not found", r.WasmId)
	}
	p := runtime.Process(t)
	if r.ExecId != "" {
		if p, err = t.Process(ctx, r.ExecId); err != nil {
			return nil, errdefs.ToGRPC(err)
		}
	}
	if err := p.Start(ctx); err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	state, err := p.State(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return &api.StartResponse{
		Pid: state.Pid,
	}, nil
}

func (l *local) Delete(ctx context.Context, r *api.DeleteTaskRequest, _ ...grpc.CallOption) (*api.DeleteResponse, error) {
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task %v not found", r.WasmId)
	}
	if err := l.monitor.Stop(t); err != nil {
		return nil, err
	}

	exit, err := l.runtime.Delete(ctx, r.WasmId)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	return &api.DeleteResponse{
		ExitStatus: exit.Status,
		ExitedAt:   ToTimestamp(exit.Timestamp),
		Pid:        exit.Pid,
	}, nil
}

func (l *local) DeleteProcess(ctx context.Context, r *api.DeleteProcessRequest, _ ...grpc.CallOption) (*api.DeleteResponse, error) {
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	process, err := t.Process(ctx, r.ExecId)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	exit, err := process.Delete(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return &api.DeleteResponse{
		Id:         r.ExecId,
		ExitStatus: exit.Status,
		ExitedAt:   ToTimestamp(exit.Timestamp),
		Pid:        exit.Pid,
	}, nil
}

func getProcessState(ctx context.Context, p runtime.Process) (*task.Process, error) {
	ctx, cancel := timeout.WithContext(ctx, stateTimeout)
	defer cancel()

	state, err := p.State(ctx)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, err
		}
		log.G(ctx).WithError(err).Errorf("get state for %s", p.ID())
	}
	status := task.StatusUnknown
	switch state.Status {
	case runtime.CreatedStatus:
		status = task.StatusCreated
	case runtime.RunningStatus:
		status = task.StatusRunning
	case runtime.StoppedStatus:
		status = task.StatusStopped
	case runtime.PausedStatus:
		status = task.StatusPaused
	case runtime.PausingStatus:
		status = task.StatusPausing
	default:
		log.G(ctx).WithField("status", state.Status).Warn("unknown status")
	}
	return &task.Process{
		ID:         p.ID(),
		Pid:        state.Pid,
		Status:     status,
		Stdin:      state.Stdin,
		Stdout:     state.Stdout,
		Stderr:     state.Stderr,
		Terminal:   state.Terminal,
		ExitStatus: state.ExitStatus,
		ExitedAt:   state.ExitedAt,
	}, nil
}

func (l *local) Get(ctx context.Context, r *api.GetRequest, _ ...grpc.CallOption) (*api.GetResponse, error) {
	task, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	p := runtime.Process(task)
	if r.ExecId != "" {
		if p, err = task.Process(ctx, r.ExecId); err != nil {
			return nil, errdefs.ToGRPC(err)
		}
	}
	t, err := getProcessState(ctx, p)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return &api.GetResponse{
		Process: t,
	}, nil
}

// only lists tasks of runtimeV2
func (l *local) List(ctx context.Context, r *api.ListTasksRequest, _ ...grpc.CallOption) (*api.ListTasksResponse, error) {
	resp := &api.ListTasksResponse{}

  tasks, err := l.runtime.Tasks(ctx, false)
  if err != nil {
    return nil, errdefs.ToGRPC(err)
  }
  addTasks(ctx, resp, tasks)

	return resp, nil
}

func addTasks(ctx context.Context, r *api.ListTasksResponse, tasks []runtime.Task) {
	for _, t := range tasks {
		tt, err := getProcessState(ctx, t)
		if err != nil {
			if !errdefs.IsNotFound(err) { // handle race with deletion
				log.G(ctx).WithError(err).WithField("id", t.ID()).Error("converting task to protobuf")
			}
			continue
		}
		r.Tasks = append(r.Tasks, tt)
	}
}

func (l *local) Kill(ctx context.Context, r *api.KillRequest, opts ...grpc.CallOption) (*types.Empty, error) {
	task, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	p := runtime.Process(task)
	if r.ExecId != "" {
		if p, err = task.Process(ctx, r.ExecId); err != nil {
			return nil, errdefs.ToGRPC(err)
		}
	}
	if err := p.Kill(ctx, r.Signal, r.All); err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return empty, nil
}

func (l *local) Pause(ctx context.Context, r *api.PauseTaskRequest, opts ...grpc.CallOption) (*types.Empty, error) {
	task, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	err = task.Pause(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return empty, nil
}

func (l *local) Resume(ctx context.Context, r *api.ResumeTaskRequest, opts ...grpc.CallOption) (*types.Empty, error) {
	task, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	err = task.Resume(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return empty, nil
}

func (l *local) ListPids(ctx context.Context, r *api.ListPidsRequest, opts ...grpc.CallOption) (*api.ListPidsResponse, error) {
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	processList, err := t.Pids(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	var processes []*task.ProcessInfo
	for _, p := range processList {
		pInfo := task.ProcessInfo{
			Pid: p.Pid,
		}
		if p.Info != nil {
			a, err := typeurl.MarshalAny(p.Info)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal process %d info: %w", p.Pid, err)
			}
			pInfo.Info = a
		}
		processes = append(processes, &pInfo)
	}
	return &api.ListPidsResponse{
		Processes: processes,
	}, nil
}

func (l *local) Update(ctx context.Context, r *api.UpdateTaskRequest, opts ...grpc.CallOption) (*types.Empty, error) {
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	if err := t.Update(ctx, anyFromPbToTypes(r.Resources), r.Annotations); err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return empty, nil
}

func (l *local) Wait(ctx context.Context, r *api.WaitRequest, opts ...grpc.CallOption) (*api.WaitResponse, error) {
	t, err := l.runtime.Get(ctx, r.WasmId)
	if err != nil {
		return nil, err
	}
	p := runtime.Process(t)
	if r.ExecId != "" {
		if p, err = t.Process(ctx, r.ExecId); err != nil {
			return nil, errdefs.ToGRPC(err)
		}
	}
	exit, err := p.Wait(ctx)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}
	return &api.WaitResponse{
		ExitStatus: exit.Status,
		ExitedAt:   ToTimestamp(exit.Timestamp),
	}, nil
}
