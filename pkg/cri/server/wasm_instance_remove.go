package server

import (
	"context"
	"fmt"
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

	wasmInstanceRemoveTimer.WithValues(wasmInstance.Runtime.Name).UpdateSince(start)

	return &runtime.RemoveContainerResponse{}, nil
}
