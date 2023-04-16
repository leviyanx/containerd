package wasmdealer

import (
	"context"
	"errors"

	api "github.com/containerd/containerd/api/services/wasmdealer/v1"
	"github.com/containerd/containerd/plugin"
	"github.com/gogo/protobuf/types"
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
			return &service{local: i.(api.WasmdealerClient)}, nil
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

func (s *service) Kill(ctx context.Context, r *api.KillRequest) (*types.Empty, error) {
	return s.local.Kill(ctx, r)
}

func (s *service) Pause(ctx context.Context, r *api.PauseTaskRequest) (*types.Empty, error) {
	return s.local.Pause(ctx, r)
}

func (s *service) Resume(ctx context.Context, r *api.ResumeTaskRequest) (*types.Empty, error) {
	return s.local.Resume(ctx, r)
}

func (s *service) ListPids(ctx context.Context, r *api.ListPidsRequest) (*api.ListPidsResponse, error) {
	return s.local.ListPids(ctx, r)
}

func (s *service) Update(ctx context.Context, r *api.UpdateTaskRequest) (*types.Empty, error) {
	return s.local.Update(ctx, r)
}

func (s *service) Wait(ctx context.Context, r *api.WaitRequest) (*api.WaitResponse, error) {
	return s.local.Wait(ctx, r)
}
