package server

import (
	"context"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *criService) RemoveWasmInstance(ctx context.Context, instance *wasminstance.WasmInstance, r *runtime.RemoveContainerRequest) (_ *runtime.RemoveContainerResponse, retErr error) {
	return &runtime.RemoveContainerResponse{}, nil
}
