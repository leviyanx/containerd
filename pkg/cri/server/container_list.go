/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package server

import (
	"time"

	"golang.org/x/net/context"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	containerstore "github.com/containerd/containerd/pkg/cri/store/container"
	"github.com/containerd/containerd/pkg/cri/store/wasminstance"
)

// ListContainers lists all containers matching the filter.
func (c *criService) ListContainers(ctx context.Context, r *runtime.ListContainersRequest) (*runtime.ListContainersResponse, error) {
	start := time.Now()
	// List all containers from store.
	containersInStore := c.containerStore.List()

	var containers []*runtime.Container
	for _, container := range containersInStore {
		containers = append(containers, toCRIContainer(container))
	}

	// List all wasm instances from store
	wasmInstancesInStore := c.wasmInstanceStore.List()
	for _, instance := range wasmInstancesInStore {
		containers = append(containers, wasmToCRIContainer(instance))
	}

	containerListTimer.UpdateSince(start)
	return &runtime.ListContainersResponse{Containers: containers}, nil
}

// toCRIContainer converts internal container object into CRI container.
func toCRIContainer(container containerstore.Container) *runtime.Container {
	status := container.Status.Get()
	return &runtime.Container{
		Id:           container.ID,
		PodSandboxId: container.SandboxID,
		Metadata:     container.Config.GetMetadata(),
		Image:        container.Config.GetImage(),
		ImageRef:     container.ImageRef,
		State:        status.State(),
		CreatedAt:    status.CreatedAt,
		Labels:       container.Config.GetLabels(),
		Annotations:  container.Config.GetAnnotations(),
	}
}

// wasmToCRIContainer converts internal wasm instance object into CRI container.
func wasmToCRIContainer(instance wasminstance.WasmInstance) *runtime.Container {
	status := instance.Status.Get()
	return &runtime.Container{
		Id:           instance.Metadata.ID,
		PodSandboxId: instance.Metadata.SandboxID,
		Metadata:     instance.Config.GetMetadata(),
		Image:        instance.Config.GetImage(),
		ImageRef:     instance.Metadata.ModuleRef,
		State:        status.State(),
		CreatedAt:    status.CreatedAt,
		Labels:       instance.Config.GetLabels(),
		Annotations:  instance.Config.GetAnnotations(),
	}
}

func (c *criService) normalizeContainerFilter(filter *runtime.ContainerFilter) {
	if cntr, err := c.containerStore.Get(filter.GetId()); err == nil {
		filter.Id = cntr.ID
	}
	if sb, err := c.sandboxStore.Get(filter.GetPodSandboxId()); err == nil {
		filter.PodSandboxId = sb.ID
	}
}

// filterCRIContainers filters CRIContainers.
func (c *criService) filterCRIContainers(containers []*runtime.Container, filter *runtime.ContainerFilter) []*runtime.Container {
	if filter == nil {
		return containers
	}

	// The containerd cri plugin supports short ids so long as there is only one
	// match. So we do a lookup against the store here if a pod id has been
	// included in the filter.
	sb := filter.GetPodSandboxId()
	if sb != "" {
		sandbox, err := c.sandboxStore.Get(sb)
		if err == nil {
			sb = sandbox.ID
		}
	}

	c.normalizeContainerFilter(filter)
	filtered := []*runtime.Container{}
	for _, cntr := range containers {
		if filter.GetId() != "" && filter.GetId() != cntr.Id {
			continue
		}
		if sb != "" && sb != cntr.PodSandboxId {
			continue
		}
		if filter.GetState() != nil && filter.GetState().GetState() != cntr.State {
			continue
		}
		if filter.GetLabelSelector() != nil {
			match := true
			for k, v := range filter.GetLabelSelector() {
				got, ok := cntr.Labels[k]
				if !ok || got != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, cntr)
	}

	return filtered
}
