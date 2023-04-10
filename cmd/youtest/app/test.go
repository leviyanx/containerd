package app

import (
	"fmt"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func Test() {
	config := runtime.ContainerConfig{}
  test(&config)
}

func test(config *runtime.ContainerConfig) {
	criMounts := config.GetMounts()
  fmt.Println(len(criMounts))
	for _, c := range criMounts {
    fmt.Println(c.HostPath)
	}
}

