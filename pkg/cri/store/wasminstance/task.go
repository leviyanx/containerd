package wasminstance

import (
	"github.com/containerd/containerd"
	containerdio "github.com/containerd/containerd/cio"
)

type WasmTask interface {
}

type wasmTask struct {
	client *containerd.Client

	io  containerdio.IO
	id  string
	pid uint32
}
