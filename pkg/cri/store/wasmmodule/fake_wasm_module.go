package wasmmodule

import "fmt"

// NewFakeStore returns a wasm module store with predefined wasm modules.
// Update is not allowed for this fake store.
func NewFakeStore(modules []WasmModule) (*Store, error) {
	s := NewStore(nil)
	for _, m := range modules {
		if err := s.Add(m); err != nil {
			return nil, fmt.Errorf("add wasm %+v: %w", m, err)
		}
	}
	return s, nil
}
