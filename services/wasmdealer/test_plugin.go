package wasmdealer

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/services/wasmdealer/v1"
	api "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/services"
)

func init() {
	plugin.Register(&plugin.Registration{
		Type: plugin.GRPCPlugin,
		ID:   "wasmdealer-test",
		Requires: []plugin.Type{
			plugin.GRPCPlugin,
		},
		InitFn: initTest,
	})
}

func initTest(ic *plugin.InitContext) (interface{}, error) {
  // test wasmdealer and its service
  // rpcCreateTest(ic)

	return nil, nil
}

func getServicesOpts(ic *plugin.InitContext) ([]containerd.ServicesOpt, error) {
	plugins, err := ic.GetByType(plugin.ServicePlugin)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plugin: %w", err)
	}

	opts := []containerd.ServicesOpt{}
	for s, fn := range map[string]func(interface{}) containerd.ServicesOpt{
		services.WasmdealerService: func(s interface{}) containerd.ServicesOpt {
			return containerd.WithWasmdealerClient(s.(wasmdealer.WasmdealerClient))
		},
	} {
		p := plugins[s]
		if p == nil {
			return nil, fmt.Errorf("service %q not found", s)
		}
		i, err := p.Instance()
		if err != nil {
			return nil, fmt.Errorf("failed to get instance of service %q: %w", s, err)
		}
		if i == nil {
			return nil, fmt.Errorf("instance of service %q not found", s)
		}
		opts = append(opts, fn(i))
	}
	return opts, nil
}

// test wasmdealer and its service
func rpcCreateTest(ic *plugin.InitContext) error {
	opts, err := getServicesOpts(ic)
	if err != nil {
		return err
	}
	client, err := containerd.New("", containerd.WithServices(opts...))
	if err != nil {
		fmt.Println("[wasmdealer-test] failed to create containerd client: ", err)
		return nil
	}

	ctx := namespaces.WithNamespace(context.Background(), "wasmdealer-test")
	response, err := client.WasmdealerService().Create(ctx, &api.CreateTaskRequest{
		WasmId: "youtest",
	})
	if err != nil {
		fmt.Println("[wasmdealer-test] failed to send request")
		return nil
	}
	fmt.Println("[wasmdealer-test] ", response.WasmId)
  return nil
}

