package wasminstance

import (
	"github.com/containerd/containerd/containers"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	"time"
)

type Metadata struct {
	// ID is the wasm instance id.
	//
	// This property is required and cannot be changed after creation.
	ID string

	// Name is the wasm instance name.
	Name string

	// Labels provide metadata extension for a wasm instance.
	//
	// These are optional and fully mutable.
	Labels map[string]string

	// WasmModuleID is the wasm module id the wasm instance belongs to.
	SandboxID string

	// Config is the CRI container config.
	Config *runtime.ContainerConfig

	// LogPath is the wasm instance log path.
	LogPath string

	// WasmModuleName is the name of the wasm module used by the wasm instance.
	WasmModuleName string

	// LogPath is the wasm instance log path.
	StopSignal string

	// Runtime specifies which runtime should be used when lanuching the wasm instance tasks.
	//
	// This property is required and immutable.
	Runtime containers.RuntimeInfo

	// CreatedAt is the time at which the container was created.
	CreatedAt time.Time

	// UpdatedAt is the time at which the container was updated.
	UpdatedAt time.Time
}
