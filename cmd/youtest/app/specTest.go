package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
)

func SpecTest() {
  ctx := namespaces.WithNamespace(context.Background(), "youtest")

  specOpts := []oci.SpecOpts{
    oci.WithProcessArgs("hello.wasm"),
	}

	spec, err := oci.GenerateSpec(ctx, nil, &containers.Container{}, specOpts...)
	if err != nil {
		fmt.Println("failed to generate spec: %w", err)
    return
	}

  PrintAsJSON(spec)
  savepath := "/home/youtirsin3/workspace/bundletest/wasmbundle/spec_test.json"
  SaveAsJSON(spec, savepath)
}

func PrintAsJSON(x interface{}) {
	b, err := json.MarshalIndent(x, "", "    ")
  if err != nil {
      fmt.Printf("can't marshal %+v as a JSON string: %v\n", x, err)
  }
	fmt.Println(string(b))
}

func SaveAsJSON(x interface{}, filename string) error {
	b, err := json.MarshalIndent(x, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, b, 0644)
}
