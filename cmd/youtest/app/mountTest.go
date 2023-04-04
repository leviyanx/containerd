package app

import (
	"fmt"
	"path/filepath"

	"github.com/containerd/containerd/mount"
)

const (
  Root = "/home/youtirsin3/workspace/bundletest/mounttest"
)

func MountTest() {
  upper :=  filepath.Join(Root, "/upper")
  lower :=  filepath.Join(Root, "/lower")
  work :=  filepath.Join(Root, "/work")
  target :=  filepath.Join(Root, "/target")

	m := mount.Mount{
		Type:   "overlay",
		Source: "overlay",
		Options: []string{
			fmt.Sprintf("lowerdir=%s", lower),
			fmt.Sprintf("upperdir=%s", upper),
			fmt.Sprintf("workdir=%s", work),
		},
	}

  if err := m.Mount(target); err != nil {
    fmt.Println("err mounting")
    fmt.Println(err.Error())
  }
}

func UnmountTest() {
  target :=  filepath.Join(Root, "/target")

  if err := mount.Unmount(target, 0); err != nil {
    fmt.Println("err unmounting")
    fmt.Println(err.Error())
  }
}

