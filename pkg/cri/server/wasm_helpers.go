package server

import (
	wasminstancestore "github.com/containerd/containerd/pkg/cri/store/wasminstance"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// copyWasmResourcesToStatus copys container resource contraints from spec to
// wasm status.
// This will need updates when new fields are added to ContainerResources.
func copyWasmResourcesToStatus(spec *runtimespec.Spec, status wasminstancestore.Status) wasminstancestore.Status {
	status.Resources = &runtime.ContainerResources{}
	if spec.Linux != nil {
		status.Resources.Linux = &runtime.LinuxContainerResources{}

		if spec.Process != nil && spec.Process.OOMScoreAdj != nil {
			status.Resources.Linux.OomScoreAdj = int64(*spec.Process.OOMScoreAdj)
		}

		if spec.Linux.Resources == nil {
			return status
		}

		if spec.Linux.Resources.CPU != nil {
			if spec.Linux.Resources.CPU.Period != nil {
				status.Resources.Linux.CpuPeriod = int64(*spec.Linux.Resources.CPU.Period)
			}
			if spec.Linux.Resources.CPU.Quota != nil {
				status.Resources.Linux.CpuQuota = *spec.Linux.Resources.CPU.Quota
			}
			if spec.Linux.Resources.CPU.Shares != nil {
				status.Resources.Linux.CpuShares = int64(*spec.Linux.Resources.CPU.Shares)
			}
			status.Resources.Linux.CpusetCpus = spec.Linux.Resources.CPU.Cpus
			status.Resources.Linux.CpusetMems = spec.Linux.Resources.CPU.Mems
		}

		if spec.Linux.Resources.Memory != nil {
			if spec.Linux.Resources.Memory.Limit != nil {
				status.Resources.Linux.MemoryLimitInBytes = *spec.Linux.Resources.Memory.Limit
			}
			if spec.Linux.Resources.Memory.Swap != nil {
				status.Resources.Linux.MemorySwapLimitInBytes = *spec.Linux.Resources.Memory.Swap
			}
		}

		if spec.Linux.Resources.HugepageLimits != nil {
			hugepageLimits := make([]*runtime.HugepageLimit, 0, len(spec.Linux.Resources.HugepageLimits))
			for _, l := range spec.Linux.Resources.HugepageLimits {
				hugepageLimits = append(hugepageLimits, &runtime.HugepageLimit{
					PageSize: l.Pagesize,
					Limit:    l.Limit,
				})
			}
			status.Resources.Linux.HugepageLimits = hugepageLimits
		}

		if spec.Linux.Resources.Unified != nil {
			status.Resources.Linux.Unified = spec.Linux.Resources.Unified
		}
	}

	if spec.Windows != nil {
		status.Resources.Windows = &runtime.WindowsContainerResources{}
		if spec.Windows.Resources != nil {
			return status
		}

		if spec.Windows.Resources.CPU != nil {
			if spec.Windows.Resources.CPU.Shares != nil {
				status.Resources.Windows.CpuShares = int64(*spec.Windows.Resources.CPU.Shares)
			}
			if spec.Windows.Resources.CPU.Count != nil {
				status.Resources.Windows.CpuCount = int64(*spec.Windows.Resources.CPU.Count)
			}
			if spec.Windows.Resources.CPU.Maximum != nil {
				status.Resources.Windows.CpuMaximum = int64(*spec.Windows.Resources.CPU.Maximum)
			}
		}

		if spec.Windows.Resources.Memory != nil {
			if spec.Windows.Resources.Memory.Limit != nil {
				status.Resources.Windows.MemoryLimitInBytes = int64(*spec.Windows.Resources.Memory.Limit)
			}
		}

		// TODO: Figure out how to get RootfsSizeInBytes
	}
	return status
}

func IsWasm(annotations map[string]string) bool {
	// if the annotation has the wasm module url item, it is a wasm module or instance
	_, urlExists := annotations["wasm.module.url"]
	return urlExists
}
