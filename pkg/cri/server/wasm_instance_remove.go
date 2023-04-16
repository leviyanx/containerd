package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	"github.com/sirupsen/logrus"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"time"
)

func (c *criService) RemoveWasmInstance(ctx context.Context, wasmInstance *wasminstance.WasmInstance, r *runtime.RemoveContainerRequest) (_ *runtime.RemoveContainerResponse, retErr error) {
	start := time.Now()
	id := wasmInstance.ID()

	// Forcibly stop the wasm instances if they are in running or unknown state
	state := wasmInstance.Status.Get().State()
	if state == runtime.ContainerState_CONTAINER_RUNNING ||
		state == runtime.ContainerState_CONTAINER_UNKNOWN {
		logrus.Infof("Forcibly stopping wasm instance %q", id)
		if err := c.stopWasmInstance(ctx, *wasmInstance, 0); err != nil {
			return nil, fmt.Errorf("failed to forcibly stop wasm instance %q: %w", id, err)
		}
	}

	// Set removing state to prevent other start/remove operations against this wasm instance
	// while it's being removed.
	if err := setWasmInstanceRemoving(wasmInstance); err != nil {
		return nil, fmt.Errorf("failed to set removing state for wasm instance %q: %w", id, err)
	}

	// Delete wasm instance
	if err := wasmInstance.Delete(ctx); err != nil {
		if !errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete wasm instance %q: %w", id, err)
		}
		log.G(ctx).Tracef("Remove called for wasm instance %q that does not exist", id)
	}

	// Delete wasm instance checkpoint(status)
	if err := wasmInstance.DeleteCheckpoint(); err != nil {
		return nil, fmt.Errorf("failed to delete checkpoint for wasm instance %q: %w", id, err)
	}

	// Delete root dir and volatile root dir
	wasmInstanceRootDir := c.getWasmInstanceRootDir(id)
	if err := ensureRemoveAll(ctx, wasmInstanceRootDir); err != nil {
		return nil, fmt.Errorf("failed to remove root dir for wasm instance %q: %w", id, err)
	}
	volatileWasmInstanceRootDir := c.getVolatileWasmInstanceRootDir(id)
	if err := ensureRemoveAll(ctx, volatileWasmInstanceRootDir); err != nil {
		return nil, fmt.Errorf("failed to remove volatile root dir for wasm instance %q: %w", id, err)
	}

	// Remove the wasm instance from store
	c.wasmInstanceStore.Delete(id)

	// Remove the wasm instance from the name index store.
	c.wasmInstanceNameIndex.ReleaseByKey(id)

	wasmInstanceRemoveTimer.WithValues(wasmInstance.Runtime.Name).UpdateSince(start)

	return &runtime.RemoveContainerResponse{}, nil
}

// setWasmInstanceRemoving sets the removing state for the given wasm instance. In removing state, the
// wasm instance can't be started or removed.
func setWasmInstanceRemoving(wasmInstance *wasminstance.WasmInstance) error {
	return wasmInstance.Status.Update(func(status wasminstance.Status) (wasminstance.Status, error) {
		// Do not remove wasm instance if it's still running or unknown.
		if status.State() == runtime.ContainerState_CONTAINER_RUNNING {
			return status, errors.New("wasm instance is still running, to stop first")
		}
		if status.State() == runtime.ContainerState_CONTAINER_UNKNOWN {
			return status, errors.New("wasm instance is in unknown state, to stop first")
		}
		if status.Starting {
			return status, errors.New("wasm instance is in starting state, can't be removed")
		}
		if status.Removing {
			return status, errors.New("wasm instance is already in removing state")
		}

		status.Removing = true
		return status, nil
	})
}
