package server

import (
	"context"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
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

	// TODO: Get wasm task

	// TODO: Handle unknown state

	// TODO: Kill the wasm task belongs to wasm instance

	// TODO: Kill the wasm instance

	// TODO: Wait for a fixed time until wasm instance stop is observed by event monitor

	return nil
}

func criWasmInstanceStateToString(state runtime.ContainerState) string {
	return runtime.ContainerState_name[int32(state)]
}
