package events

import "time"

type WasmTaskExit struct {
	WasmInstanceID       string
	ID                   string
	Pid                  uint32
	ExitStatus           uint32
	ExitedAt             time.Time
	XXX_NoUnkeyedLiteral struct{}
	XXX_unrecognized     []byte
	XXX_sizecache        int32
}
