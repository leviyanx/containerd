package server

import (
	"context"

	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
	"github.com/containerd/containerd/pkg/cri/store/wasmmodule"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *criService) WasmInstanceStatus(ctx context.Context, wasmInstance *wasminstance.WasmInstance, r *runtime.ContainerStatusRequest) (*runtime.ContainerStatusResponse, error) {
	module := wasmInstance.WasmModule
	status := wasmToCRIContainerStatus(wasmInstance, &module)

	// for now, verbose information not supported, because we only need the status api for removing
	// TODO (Youtirsin): impl to offer verbose informations
	info := map[string]string{}
	return &runtime.ContainerStatusResponse{
		Status: status,
		Info:   info,
	}, nil
}

func wasmToCRIContainerStatus(wasmInstance *wasminstance.WasmInstance, module *wasmmodule.WasmModule) *runtime.ContainerStatus {
	meta := wasmInstance.Metadata
	status := wasmInstance.Status.Get()
	reason := status.Reason
	if status.State() == runtime.ContainerState_CONTAINER_EXITED && reason == "" {
		if status.ExitCode == 0 {
			reason = completeExitReason
		} else {
			reason = errorExitReason
		}
	}

	// If container is in the created state, not set started and finished unix timestamps
	var st, ft int64
	switch status.State() {
	case runtime.ContainerState_CONTAINER_RUNNING:
		// If container is in the running state, set started unix timestamps
		st = status.StartedAt
	case runtime.ContainerState_CONTAINER_EXITED, runtime.ContainerState_CONTAINER_UNKNOWN:
		st, ft = status.StartedAt, status.FinishedAt
	}

	return &runtime.ContainerStatus{
		Id:         meta.ID,
		Metadata:   meta.Config.GetMetadata(),
		State:      status.State(),
		CreatedAt:  status.CreatedAt,
		StartedAt:  st,
		FinishedAt: ft,
		ExitCode:   status.ExitCode,
		// TODO (Youtirsin): ensure this is valid
		Image:       meta.Config.Image,
		ImageRef:    meta.ModuleRef,
		Reason:      reason,
		Message:     status.Message,
		Labels:      meta.Config.GetLabels(),
		Annotations: meta.Config.GetAnnotations(),
		Mounts:      meta.Config.GetMounts(),
		LogPath:     meta.LogPath,
		Resources:   status.Resources,
	}
}
