package wasmmodules

import (
	"github.com/opencontainers/go-digest"
)

type WasmModuleSpec struct {
	// Basic info >>>
	Author string
	// <<< Basic info

	// Descriptor >>>
	// Digest is the digest of the wasm module
	Digest digest.Digest

	// Size is the size of the wasm module
	Size int64

	// Name of the wasm runtime running the module
	Runtime string

	// URL specifies the URL from which this wasm module MAY be downloaded
	URL string

	// Annotations contains arbitrary metadata relating to the targeted wasm module.
	Annotations map[string]string
	// <<< Descriptor

	// Config >>>
	// Cmd defines the default command for the wasm module / wasm instance
	Cmd []string
	// <<< Config
}
