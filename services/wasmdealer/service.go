package wasmdealer

import (
	"context"
	"errors"

	api "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/plugin"
	"google.golang.org/grpc"
)

func init() {
	plugin.Register(&plugin.Registration{
		Type: plugin.GRPCPlugin,
		ID:   "wasmdealer",
		Requires: []plugin.Type{
			plugin.ServicePlugin,
		},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			plugins, err := ic.GetByType(plugin.ServicePlugin)
			if err != nil {
				return nil, err
			}
			p, ok := plugins["wasmdealer-service"]
			if !ok {
				return nil, errors.New("wasmdealer service not found")
			}
			i, err := p.Instance()
			if err != nil {
				return nil, err
			}
      return &service {local: i.(api.WasmdealerClient)}, nil
		},
	})
}

type service struct {
  local api.WasmdealerClient
}

func (s *service) Register(server *grpc.Server) error {
	api.RegisterWasmdealerServer(server, s)
	return nil
}

func (s *service) Create(ctx context.Context, r *api.CreateTaskRequest) (*api.CreateTaskResponse, error) {
	return s.local.Create(ctx, r)
}

