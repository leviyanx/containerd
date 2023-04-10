package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	containerdio "github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	sandboxstore "github.com/containerd/containerd/pkg/cri/store/sandbox"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	ctrdutil "github.com/containerd/containerd/pkg/cri/util"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"time"
)

func (c *criService) StartWasmInstance(ctx context.Context, wasmInstance *wasminstance.WasmInstance, r *runtime.StartContainerRequest) (retRes *runtime.StartContainerResponse, retErr error) {
	id := wasmInstance.ID()
	meta := wasmInstance.Metadata
	config := wasmInstance.Config

	// Set starting state to prevent other start/remove operations against this container
	// while it's being started.
	if err := setWasmInstanceStarting(*wasmInstance); err != nil {
		return nil, fmt.Errorf("failed to set starting state for wasm instance %q: %w", id, err)
	}
	defer func() {
		if retErr != nil {
			// Set wasm instance to exited if fail to start.
			if err := wasmInstance.Status.UpdateSync(func(status wasminstance.Status) (wasminstance.Status, error) {
				status.Pid = 0
				status.FinishedAt = time.Now().UnixNano()
				status.ExitCode = errorStartExitCode
				status.Reason = errorStartReason
				status.Message = retErr.Error()
				return status, nil
			}); err != nil {
				log.G(ctx).WithError(err).Errorf("failed to set start failure state for wasm instance %q", id)
			}
		}
		if err := resetWasmInstanceStarting(*wasmInstance); err != nil {
			log.G(ctx).WithError(err).Errorf("failed to reset starting state for wasm instance %q", id)
		}
	}()

	// Get sandbox config from sandbox store.
	sandbox, err := c.sandboxStore.Get(meta.SandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox %q not found: %w", meta.SandboxID, err)
	}
	sandboxID := meta.SandboxID
	if sandbox.Status.Get().State != sandboxstore.StateReady {
		return nil, fmt.Errorf("sandbox %q is not running", sandboxID)
	}

	// TODO: recheck target wasm instance validity in Linux namespace options.

	ioCreation := func(id string) (_ containerdio.IO, err error) {
		stdoutWC, stderrWC, err := c.createContainerLoggers(meta.LogPath, config.GetTty())
		if err != nil {
			return nil, fmt.Errorf("failed to create wasm instance loggers: %w", err)
		}
		wasmInstance.IO.AddOutput("log", stdoutWC, stderrWC)
		wasmInstance.IO.Pipe()
		return wasmInstance.IO, nil
	}

	_, err = c.getSandboxRuntime(sandbox.Config, sandbox.Metadata.RuntimeHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox runtime: %w", err)
	}

	// Create wasm instance task and delete task when encountering error.
	wasmTask, err := wasmInstance.NewTask(ctx, c.client, ioCreation)
	if err != nil {
		return nil, fmt.Errorf("failed to create wasm instance task: %w", err)
	}
	defer func() {
		if retErr != nil {
			deferCtx, deferCancel := ctrdutil.DeferContext()
			defer deferCancel()
			// It's possible that task is deleted by event monitor.
			// NOTE: Delete wasm task don't need to call WithNRISandboxDelete, because we don't create NRI for wasm task
			if _, err := wasmTask.Delete(deferCtx, containerd.WithProcessKill); err != nil && !errdefs.IsNotFound(err) {
				log.G(ctx).WithError(err).Errorf("Failed to delete wasm task %q", id)
			}
		}
	}()

	// Wait task: wait is a long running operation, no timeout needed.
	exitCh, err := wasmTask.Wait(ctrdutil.NamespacedContext())
	if err != nil {
		return nil, fmt.Errorf("failed to wait for wasm instance task %q: %w", id, err)
	}
	// NOTE: We don't need nri

	// Start wasm instance task
	if err := wasmTask.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start wasm instance task %q: %w", id, err)
	}

	// Update wasm instance start timestamp.
	if err := wasmInstance.Status.Update(func(status wasminstance.Status) (wasminstance.Status, error) {
		status.Pid = wasmTask.Pid()
		status.StartedAt = time.Now().UnixNano()
		return status, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to update wasm instance %q state: %w", id, err)
	}

	// It handles the TaskExit event and updates the status of the wasm instance after this.
	c.eventMonitor.startWasmInstanceExitMonitor(context.Background(), id, wasmTask.Pid(), exitCh)

	return &runtime.StartContainerResponse{}, nil
}

func setWasmInstanceStarting(wasmInstance wasminstance.WasmInstance) error {
	return wasmInstance.Status.Update(func(status wasminstance.Status) (wasminstance.Status, error) {
		// Return error if wasm instance is not in created state.
		if status.State() != runtime.ContainerState_CONTAINER_CREATED {
			return status, fmt.Errorf("wasm instance is in %s state", criContainerStateToString(status.State()))
		}

		// Do not start the wasm instance when there is a removal in progress.
		if status.Removing {
			return status, errors.New("wasm instance is in removing state, can't be started")
		}
		if status.Starting {
			return status, errors.New("wasm instance is already in starting state")
		}
		status.Starting = true
		return status, nil
	})
}

// resetWasmInstanceStarting resets the wasm instance starting state on start failure. So
// that we could remove the wasm instance later.
func resetWasmInstanceStarting(wasmInstance wasminstance.WasmInstance) error {
	return wasmInstance.Status.Update(func(status wasminstance.Status) (wasminstance.Status, error) {
		status.Starting = false
		return status, nil
	})
}
