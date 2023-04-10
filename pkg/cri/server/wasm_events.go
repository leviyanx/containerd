package server

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	containerdio "github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	ctrdutil "github.com/containerd/containerd/pkg/cri/util"
	"github.com/sirupsen/logrus"
	"time"
)

// startWasmInstanceExitMonitor starts an exit monitor for a given wasm instance.
func (em *eventMonitor) startWasmInstanceExitMonitor(ctx context.Context, id string, pid uint32, exitCh <-chan containerd.ExitStatus) <-chan struct{} {
	stopCh := make(chan struct{})
	go func() {
		defer close(stopCh)
		select {
		case exitRes := <-exitCh:
			exitStatus, exitedAt, err := exitRes.Result()
			if err != nil {
				logrus.WithError(err).Errorf("failed to get task exit status for %q", id)
				exitStatus = unknownExitCode
				exitedAt = time.Now()
			}

			// NOTE: maybe don't need event here
			e := &eventtypes.WasmTaskExit{
				WasmInstanceID: id,
				ID:             id,
				Pid:            pid,
				ExitStatus:     exitStatus,
				ExitedAt:       exitedAt,
			}
			logrus.Debugf("WasmTaskExit exit event: %+v", e)

			err = func() error {
				dctx := ctrdutil.NamespacedContext()
				dctx, dcancel := context.WithTimeout(dctx, handleEventTimeout)
				defer dcancel()

				// handle wasm instance exit
				wasmInstance, err := em.c.wasmInstanceStore.Get(e.ID)
				if err == nil {
					if err := handleWasmInstanceExit(dctx, e, wasmInstance); err != nil {
						return err
					}
					return nil
				} else if !errdefs.IsNotFound(err) {
					return fmt.Errorf("failed to get wasm instance %q: %w", e.ID, err)
				}
				return nil
			}()
			if err != nil {
				logrus.WithError(err).Errorf("failed to handle wasm instance WasmTaskExit event %+v", e)
				em.backOff.enBackOff(id, e)
			}
			return
		case <-ctx.Done():
		}
	}()
	return stopCh
}

// handleWasmInstanceExit handles the WasmTaskExit event for wasm instance.
func handleWasmInstanceExit(ctx context.Context, e *eventtypes.WasmTaskExit, wasmInstance wasminstance.WasmInstance) error {
	// Attach wasm instance IO so that `Delete` could cleanup the stream properly
	wasmTask, err := wasmInstance.Task(ctx,
		func(*containerdio.FIFOSet) (containerdio.IO, error) {
			// We can't directly return wasmInstance.IO here, because
			// even if wasmInstance.IO is nil, the cio.IO interface
			// is not.
			// See https://tour.golang.org/methods/12:
			//   Note that an interface value that holds a nil
			//   concrete value is itself non-nil.
			if wasmInstance.IO != nil {
				return wasmInstance.IO, nil
			}
			return nil, nil
		},
	)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to load task for wasm instance")
		}
	} else {
		// TODO(leviyan): [P1] This may block the loop, we may want to spawn a worker (refer to handleContainerExit)
		// NOTE: Delete wasm task don't need to call WithNRISandboxDelete, because we don't create NRI for wasm task
		if _, err = wasmTask.Delete(ctx, containerd.WithProcessKill); err != nil {
			if !errdefs.IsNotFound(err) {
				return fmt.Errorf("failed to stop wasm instance: %w", err)
			}
			// Move on to make sure wasm instance status is updated.
		}
	}

	// Update wasm instance
	err = wasmInstance.Status.UpdateSync(func(status wasminstance.Status) (wasminstance.Status, error) {
		if status.FinishedAt == 0 {
			status.Pid = 0
			status.FinishedAt = e.ExitedAt.UnixNano()
			status.ExitCode = int32(e.ExitStatus)
		}

		// Unknown status can only transit to EXITED state, so we need to handle
		// unknown state here.
		if status.Unknown {
			logrus.Debugf("Wasm instance %q transited from UNKNOWN to EXITED", wasmInstance.ID())
			status.Unknown = false
		}
		return status, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update wasm instance state: %w", err)
	}

	// Using channel to propagate the information of wasm instance stop
	wasmInstance.Stop()
	return nil
}
