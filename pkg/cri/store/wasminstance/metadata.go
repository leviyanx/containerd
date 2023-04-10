package wasminstance

import (
	"google.golang.org/protobuf/types/known/anypb"
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

	// WasmInstanceRootDir is the root directory of the wasm instance.
	WasmInstanceRootDir string

	// WasmModuleName is the name of the wasm module used by the wasm instance.
	WasmModuleName string

	// StopSignal is the system call signal that will be sent to the wasm instance to exit.
	StopSignal string

	// Runtime specifies which runtime should be used when lanuching the wasm instance tasks.
	//
	// This property is required and immutable.
	Runtime RuntimeInfo
	// Spec is used to create tasks from the wasm instance.
	Spec *anypb.Any

	// CreatedAt is the time at which the container was created.
	CreatedAt time.Time

	// UpdatedAt is the time at which the container was updated.
	UpdatedAt time.Time
}

// RuntimeInfo holds runtime specific information
type RuntimeInfo struct {
	Name    string
	Options *anypb.Any
}
