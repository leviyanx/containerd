package server

import (
	"context"
	"fmt"
	eventtypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	ctrdutil "github.com/containerd/containerd/pkg/cri/util"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"time"
)

func (c *criService) StopWasmInstance(ctx context.Context, wasmInstance *wasminstance.WasmInstance, r *runtime.StopContainerRequest) (*runtime.StopContainerResponse, error) {
	start := time.Now()

	if err := c.stopWasmInstance(ctx, *wasmInstance, time.Duration(r.GetTimeout())*time.Second); err != nil {
		return nil, err
	}

	wasmInstanceStopTimer.WithValues(wasmInstance.Runtime.Name).UpdateSince(start)

	return &runtime.StopContainerResponse{}, nil
}

func (c *criService) stopWasmInstance(ctx context.Context, wasmInstance wasminstance.WasmInstance, timeout time.Duration) error {
	id := wasmInstance.ID()

	// Return without error if wasm instance is not running. This makes sure that
	// stop only takes real action after the wasm instance is started.
	state := wasmInstance.Status.Get().State()
	if state != runtime.ContainerState_CONTAINER_RUNNING &&
		state != runtime.ContainerState_CONTAINER_UNKNOWN {
		log.G(ctx).Infof("Wasm instance to stop %q must be in running or unknown state, current state %q",
			id, criWasmInstanceStateToString(state))
		return nil
	}

	// Get wasm task
	wasmTask, err := wasmInstance.Task(ctx, nil)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to get task for wasm instance %q: %w", id, err)
		}
		// Don't return for unknown state, some cleanup needs to be done.
		if state == runtime.ContainerState_CONTAINER_UNKNOWN {
			return cleanupUnknownWasmInstance(ctx, id, wasmInstance)
		}
		return nil
	}

	// Handle unknown state
	if state == runtime.ContainerState_CONTAINER_UNKNOWN {
		// Start an exit handler for wasm instances in unknown state.
		waitCtx, waitCancel := context.WithCancel(ctrdutil.NamespacedContext())
		defer waitCancel()
		exitCh, err := wasmTask.Wait(waitCtx)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return fmt.Errorf("failed to wait for wasm instance %q: %w", id, err)
			}
			return cleanupUnknownWasmInstance(ctx, id, wasmInstance)
		}

		exitCtx, exitCancel := context.WithCancel(context.Background())
		stopCh := c.eventMonitor.startWasmInstanceExitMonitor(exitCtx, id, wasmTask.Pid(), exitCh)
		defer func() {
			exitCancel()
			// This ensures that the exit monitor is stopped before `Wait` is canceled,
			// so no exit event is generated because of the `Wait` cancellation.
			<-stopCh
		}()
	}

	// TODO: Kill the wasm task belongs to wasm instance

	// TODO: Kill the wasm instance

	// TODO: Wait for a fixed time until wasm instance stop is observed by event monitor

	return nil
}

// cleanupUnknownWasmInstance cleans up stopped wasm instance in unknown state.
func cleanupUnknownWasmInstance(ctx context.Context, id string, wasmInstance wasminstance.WasmInstance) error {
	// Reuse handleWasmInstanceExit to do the cleanup
	return handleWasmInstanceExit(ctx, &eventtypes.WasmTaskExit{
		WasmInstanceID: id,
		ID:             id,
		Pid:            0,
		ExitStatus:     unknownExitCode,
		ExitedAt:       time.Now(),
	}, wasmInstance)
}

func criWasmInstanceStateToString(state runtime.ContainerState) string {
	return runtime.ContainerState_name[int32(state)]
}
