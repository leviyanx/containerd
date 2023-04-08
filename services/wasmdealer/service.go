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

func (s *service) Start(ctx context.Context, r *api.StartRequest) (*api.StartResponse, error) {
  return s.local.Start(ctx, r)
}

func (s *service) Delete(ctx context.Context, r *api.DeleteTaskRequest) (*api.DeleteResponse, error) {
  return s.local.Delete(ctx, r)
}

func (s *service) DeleteProcess(ctx context.Context, r *api.DeleteProcessRequest) (*api.DeleteResponse, error) {
  return s.local.DeleteProcess(ctx, r)
}

func (s *service) Get(ctx context.Context, r *api.GetRequest) (*api.GetResponse, error) {
  return s.local.Get(ctx, r)
}

func (s *service) List(ctx context.Context, r *api.ListTasksRequest) (*api.ListTasksResponse, error) {
  return s.local.List(ctx, r)
}

