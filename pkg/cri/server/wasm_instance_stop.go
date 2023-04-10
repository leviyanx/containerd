package server

import (
	"context"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *criService) StopWasmInstance(ctx context.Context, wasmInstance *wasminstance.WasmInstance, r *runtime.StopContainerRequest) (*runtime.StopContainerResponse, error) {

	return &runtime.StopContainerResponse{}, nil
}
