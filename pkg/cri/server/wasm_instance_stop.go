package server

import (
	"context"
	"fmt"
	eventtypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	ctrdutil "github.com/containerd/containerd/pkg/cri/util"
	"github.com/moby/sys/signal"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"sync/atomic"
	"syscall"
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

	// Kill the wasm task belonging to wasm instance
	// We only need to kill the task. The event handler will Delete the task from containerd
	// after i handles the Exited event.
	if timeout > 0 {
		stopSignal := "SIGTERM"
		if wasmInstance.StopSignal != "" {
			stopSignal = wasmInstance.StopSignal
		} else {
			// The wasm module may have been deleted, and the `StopSignal` field is
			// just introduced to handle that.
			// However, for wasm instances created before the `StopSignal` field is
			// introduced, still try to get the stop signal from the wasm module config.
			// If the wasm module has been deleted, logging an error and using the
			// default SIGTERM is still better than returning error and leaving
			// the wasm instance unstoppable. (See issue #990)
			// TODO(levi-yan): set the stop signal when pulling the wasm module \
			// and set the stop signal when creating the wasm instance.
			wasmModule, err := c.wasmModuleStore.Get(wasmInstance.WasmModuleName)
			if err != nil {
				if !errdefs.IsNotFound(err) {
					return fmt.Errorf("failed to get wasm module %q: %w", wasmInstance.WasmModuleName, err)
				}
				log.G(ctx).Warningf("Wasm module %q not found, stop wasm instance with signal %q", wasmInstance.WasmModuleName, stopSignal)
			} else {
				if wasmModule.WasmModuleSpec.StopSignal != "" {
					stopSignal = wasmModule.WasmModuleSpec.StopSignal
				}
			}
		}
		sig, err := signal.ParseSignal(stopSignal)
		if err != nil {
			return fmt.Errorf("failed to parse stop signal %q for wasm instance %q: %w", stopSignal, id, err)
		}

		var sswt bool
		if wasmInstance.IsStopSignaledWithTimeout == nil {
			log.G(ctx).Infof("Unable to ensure stop signal %v was not sent twice to wasm instance %v", sig, id)
			sswt = true
		} else {
			sswt = atomic.CompareAndSwapUint32(wasmInstance.IsStopSignaledWithTimeout, 0, 1)
		}

		if sswt {
			log.G(ctx).Infof("Stop wasm instance %q with signal %v", id, sig)
			if err = wasmTask.Kill(ctx, sig); err != nil && !errdefs.IsNotFound(err) {
				return fmt.Errorf("failed to stop wasm instance %q: %w", id, err)
			}
		} else {
			log.G(ctx).Infof("Skipping the sending of signal %v to wasm instance %q because a prior stop with"+
				"timeout>0 request already send the signal", sig, id)
		}

		sigTermCtx, sigTermCtxCancel := context.WithTimeout(ctx, timeout)
		defer sigTermCtxCancel()
		err = c.waitWasmInstanceStop(sigTermCtx, wasmInstance)
		if err == nil {
			// The wasm instance is stopped on first signal no need for SIGKILL
			return nil
		}
		// If the parent context was cancelled or exceeded return immediately
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// sigTermCtx was exceeded. Send SIGKILL
		log.G(ctx).Debugf("Stop wasm instance %q with signal %v time out", id, sig)
	}

	// Send SIGTERM doesn't take effect, send SIGKILL
	log.G(ctx).Infof("Kill wasm instance %q", id)
	if err = wasmTask.Kill(ctx, syscall.SIGKILL); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("failed to kill wasm instance %q: %w", id, err)
	}

	// Wait for a fixed time until wasm instance stop is observed by event monitor
	err = c.waitWasmInstanceStop(ctx, wasmInstance)
	if err != nil {
		return fmt.Errorf("an error occurs during waiting for wasm instance %q to be killed: %w", id, err)
	}
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

// waitWasmInstanceStop waits for the wasm instance to stopped until context is
// cancelled or the context deadline is exceeded.
func (c *criService) waitWasmInstanceStop(ctx context.Context, wasmInstance wasminstance.WasmInstance) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait wasm instance %q: %w", wasmInstance.ID(), ctx.Err())
	case <-wasmInstance.Stopped():
		return nil
	}
}

func criWasmInstanceStateToString(state runtime.ContainerState) string {
	return runtime.ContainerState_name[int32(state)]
}
