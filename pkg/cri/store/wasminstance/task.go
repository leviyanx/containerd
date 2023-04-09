package wasminstance

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	wasmdealer "github.com/containerd/containerd/api/services/wasmdealer/v1"
	containerdio "github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/plugin"
	"strings"
	"syscall"
	"time"
)

type WasmTask interface {
	containerd.Process
}

type wasmTask struct {
	client *containerd.Client

	io  containerdio.IO
	id  string
	pid uint32
}

func (t *wasmTask) ID() string {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Pid() uint32 {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Start(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Kill(ctx context.Context, signal syscall.Signal, opts ...containerd.KillOpts) error {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Wait(ctx context.Context) (<-chan containerd.ExitStatus, error) {
	c := make(chan containerd.ExitStatus, 1)
	go func() {
		defer close(c)

		response, err := t.client.WasmdealerService().Wait(ctx, &wasmdealer.WaitRequest{
			WasmId: t.id,
		})
		if err != nil {
			c <- *containerd.NewExitStatus(containerd.UnknownExitStatus, time.Now(), err)
			return
		}
		c <- *containerd.NewExitStatus(response.GetExitStatus(), response.GetExitedAt().AsTime(), nil)
	}()
	return c, nil
}

func (t *wasmTask) CloseIO(ctx context.Context, opts ...containerd.IOCloserOpts) error {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Resize(ctx context.Context, w, h uint32) error {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) IO() containerdio.IO {
	//TODO implement me
	panic("implement me")
}

func (t *wasmTask) Status(ctx context.Context) (containerd.Status, error) {
	r, err := t.client.WasmdealerService().Get(ctx, &wasmdealer.GetRequest{
		WasmId: t.id,
	})
	if err != nil {
		return containerd.Status{}, errdefs.FromGRPC(err)
	}

	return containerd.Status{
		Status:     containerd.ProcessStatus(strings.ToLower(r.GetProcess().Status.String())),
		ExitStatus: r.GetProcess().ExitStatus,
		ExitTime:   r.GetProcess().ExitedAt,
	}, nil
}

// Delete deletes the task and its runtime state
// it returns the exit status of the task and any errors that were encountered
// during cleanup
func (t *wasmTask) Delete(ctx context.Context, opts ...containerd.ProcessDeleteOpts) (*containerd.ExitStatus, error) {
	for _, o := range opts {
		if err := o(ctx, t); err != nil {
			return nil, err
		}
	}

	status, err := t.Status(ctx)
	if err != nil && errdefs.IsNotFound(err) {
		return nil, err
	}
	switch status.Status {
	case containerd.Stopped, containerd.Unknown:
	case containerd.Created:
		if t.client.Runtime() == fmt.Sprintf("%s.%s", plugin.RuntimePlugin, "Windows") {
			// On windows Created is akin to Stopped
			break
		}
		if t.pid == 0 {
			// allow for deletion of created tasks with PID 0
			// https://github.com/containerd/containerd/issues/7357
			break
		}
		fallthrough
	default:
		return nil, fmt.Errorf("task must be stopped before deletion: %s: %w", status.Status, errdefs.ErrFailedPrecondition)
	}
	if t.io != nil {
		t.io.Close()
		t.io.Cancel()
		t.io.Wait()
	}

	// Delete the task
	response, err := t.client.WasmdealerService().Delete(ctx, &wasmdealer.DeleteTaskRequest{
		WasmId: t.id,
	})
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}
	// Only cleanup the IO after a successful delete
	if t.io != nil {
		t.io.Close()
	}

	exitStatus := containerd.NewExitStatus(response.GetExitStatus(), response.GetExitedAt().AsTime(), nil)
	return exitStatus, nil
}
