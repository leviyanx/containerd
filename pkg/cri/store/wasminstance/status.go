package wasminstance

import (
	"encoding/json"
	"fmt"
	"github.com/containerd/continuity"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"path/filepath"
	"sync"
)

// statusVersion is current version of wasm instance status.
const statusVersion = "v1"

// versionedStatus is the internal used versioned wasm instance status.
// nolint
type versionedStatus struct {
	// Version indicates the version of the versioned wasm instance status.
	Version string
	Status
}

// Status is the status of a wasm instance.
type Status struct {
	// Pid is the init process id of the wasm instance.
	Pid uint32

	// CreatedAt is the created timestamp.
	CreatedAt int64
	// StartedAt is the started timestamp.
	StartedAt int64
	// FinishedAt is the finished timestamp.
	FinishedAt int64

	// ExitCode is the wasm instance exit code.
	ExitCode int32

	// CamelCase string explaining why wasm instance is in its current state.
	Reason string
	// Human-readable message indicating details about why the wasm instance is
	// in its current state.
	Message string

	// Starting indicates that the wasm instance is in starting state.
	Starting bool
	// Running indicates that the wasm instance is in running state.
	Removing bool
	// Unknown indicates that the wasm instance status is not fully loaded.
	Unknown bool

	// Resources has wasm runtime resource constraints.
	Resources *runtime.ContainerResources
}

func (s Status) State() runtime.ContainerState {
	if s.Unknown {
		return runtime.ContainerState_CONTAINER_UNKNOWN
	}
	if s.FinishedAt != 0 {
		return runtime.ContainerState_CONTAINER_EXITED
	}
	if s.StartedAt != 0 {
		return runtime.ContainerState_CONTAINER_RUNNING
	}
	if s.CreatedAt != 0 {
		return runtime.ContainerState_CONTAINER_CREATED
	}
	return runtime.ContainerState_CONTAINER_UNKNOWN
}

func (s *Status) encode() ([]byte, error) {
	return json.Marshal(&versionedStatus{
		Version: statusVersion,
		Status:  *s,
	})
}

// UpdateFunc is function used to update the wasm instance status. If there is
// an error, the update wil be rolled back.
type UpdateFunc func(Status) (Status, error)

type StatusStorage interface {
	// Get a wasm instance status.
	Get() Status

	// UpdateSync updates the wasm instance status and then on disk checkpoint.
	// Note that the update MUST be applied in one transaction.
	UpdateSync(UpdateFunc) error

	// Update the wasm instance status. Note that the update MUST be applied
	// in one transaction.
	Update(UpdateFunc) error
}

// StoreStatus creates the storage containing the passed in wasm instance status with the specified id.
// The status MUST be created in one transaction.
func StoreStatus(root, id string, status Status) (StatusStorage, error) {
	data, err := status.encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode status: %v", err)
	}
	// id temporarily not used because the path of root contains the id
	path := filepath.Join(root, "status")
	if err := continuity.AtomicWriteFile(path, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to checkoutpoin status to %q: %v", path, err)
	}
	return &statusStorage{
		path:   path,
		status: status,
	}, nil
}

type statusStorage struct {
	sync.RWMutex
	path   string
	status Status
}

func (s *statusStorage) Get() Status {
	s.RLock()
	defer s.RUnlock()
	return deepCopy(s.status)
}

// UpdateSync updates the wasm instance status and then on disk checkpoint.
func (s *statusStorage) UpdateSync(u UpdateFunc) error {
	s.Lock()
	defer s.Unlock()

	newStatus, err := u(s.status)
	if err != nil {
		return err
	}
	data, err := newStatus.encode()
	if err != nil {
		return fmt.Errorf("failed to encode status: %v", err)
	}
	if err := continuity.AtomicWriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("failed to checkpoint status to %q: %v", s.path, err)
	}
	s.status = newStatus
	return nil
}

func deepCopy(s Status) Status {
	copiedS := s
	// Resources is the only field that is a pointer, and therefore needs
	// a manual deep copy.
	// This will need updates when new fields are added to ContainerResources.
	if s.Resources == nil {
		return copiedS
	}

	copiedS.Resources = &runtime.ContainerResources{}
	if s.Resources != nil && s.Resources.Linux != nil {
		hugepageLimites := make([]*runtime.HugepageLimit, 0, len(s.Resources.Linux.HugepageLimits))
		for _, l := range s.Resources.Linux.HugepageLimits {
			if l != nil {
				hugepageLimites = append(hugepageLimites, &runtime.HugepageLimit{
					PageSize: l.PageSize,
					Limit:    l.Limit,
				})
			}
		}
		copiedS.Resources = &runtime.ContainerResources{
			Linux: &runtime.LinuxContainerResources{
				CpuPeriod:              s.Resources.Linux.CpuPeriod,
				CpuQuota:               s.Resources.Linux.CpuQuota,
				CpuShares:              s.Resources.Linux.CpuShares,
				CpusetCpus:             s.Resources.Linux.CpusetCpus,
				CpusetMems:             s.Resources.Linux.CpusetMems,
				MemoryLimitInBytes:     s.Resources.Linux.MemoryLimitInBytes,
				MemorySwapLimitInBytes: s.Resources.Linux.MemorySwapLimitInBytes,
				OomScoreAdj:            s.Resources.Linux.OomScoreAdj,
				Unified:                s.Resources.Linux.Unified,
				HugepageLimits:         hugepageLimites,
			},
		}
	}

	if s.Resources != nil && s.Resources.Windows != nil {
		copiedS.Resources = &runtime.ContainerResources{
			Windows: &runtime.WindowsContainerResources{
				CpuShares:          s.Resources.Windows.CpuShares,
				CpuCount:           s.Resources.Windows.CpuCount,
				CpuMaximum:         s.Resources.Windows.CpuMaximum,
				MemoryLimitInBytes: s.Resources.Windows.MemoryLimitInBytes,
				RootfsSizeInBytes:  s.Resources.Windows.RootfsSizeInBytes,
			},
		}
	}

	return copiedS
}

// Update the container status.
func (s *statusStorage) Update(u UpdateFunc) error {
	s.Lock()
	defer s.Unlock()

	newStatus, err := u(s.status)
	if err != nil {
		return err
	}
	s.status = newStatus
	return nil
}
