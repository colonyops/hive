package daemon

import (
	"context"
	"sync"

	"connectrpc.com/connect"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/hive/projectapi"
)

type projectServer struct {
	api projectapi.API
	// writeMu is the session server's: a project create runs session
	// creates, which write to the same shared stdout a session create points
	// at its stream.
	writeMu *sync.Mutex
}

var _ daemonv1connect.ProjectServiceHandler = (*projectServer)(nil)

func (s *projectServer) ListProjects(ctx context.Context, _ *connect.Request[daemonv1.ListProjectsRequest]) (*connect.Response[daemonv1.ListProjectsResponse], error) {
	all, err := s.api.ListProjects(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	out := make([]*daemonv1.Project, 0, len(all))
	for _, p := range all {
		out = append(out, wire.ProjectToProto(p))
	}
	return connect.NewResponse(&daemonv1.ListProjectsResponse{Projects: out}), nil
}

func (s *projectServer) ProjectDefinitions(ctx context.Context, _ *connect.Request[daemonv1.ProjectDefinitionsRequest]) (*connect.Response[daemonv1.ProjectDefinitionsResponse], error) {
	defs, err := s.api.ProjectDefinitions(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	out := make([]*daemonv1.ProjectDefinition, 0, len(defs))
	for _, d := range defs {
		out = append(out, wire.DefinitionToProto(d))
	}
	return connect.NewResponse(&daemonv1.ProjectDefinitionsResponse{Definitions: out}), nil
}

func (s *projectServer) DefaultCloneStrategy(ctx context.Context, req *connect.Request[daemonv1.DefaultCloneStrategyRequest]) (*connect.Response[daemonv1.DefaultCloneStrategyResponse], error) {
	strategy, err := s.api.DefaultCloneStrategy(ctx, req.Msg.GetRemote())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.DefaultCloneStrategyResponse{CloneStrategy: strategy}), nil
}

func (s *projectServer) CreateProject(ctx context.Context, req *connect.Request[daemonv1.CreateProjectRequest], stream *connect.ServerStream[daemonv1.CreateProjectResponse]) error {
	opts := wire.CreateProjectOptionsFromProto(req.Msg)
	var sendMu sync.Mutex
	opts.OnStep = func(step projectsvc.Step) {
		sendMu.Lock()
		defer sendMu.Unlock()
		// A client that went away stops reading; the create still finishes.
		_ = stream.Send(&daemonv1.CreateProjectResponse{Event: &daemonv1.CreateProjectResponse_Step{Step: wire.StepToProto(step)}})
	}
	s.writeMu.Lock()
	p, err := s.api.CreateProject(ctx, opts)
	s.writeMu.Unlock()
	if err != nil {
		return wire.ToConnectError(err)
	}
	return stream.Send(&daemonv1.CreateProjectResponse{Event: &daemonv1.CreateProjectResponse_Project{Project: wire.ProjectToProto(p)}})
}

func (s *projectServer) ProjectRisk(ctx context.Context, req *connect.Request[daemonv1.ProjectRiskRequest]) (*connect.Response[daemonv1.ProjectRiskResponse], error) {
	risks, err := s.api.ProjectRisk(ctx, req.Msg.GetRef())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	out := make([]*daemonv1.ProjectRepoRisk, 0, len(risks))
	for _, r := range risks {
		out = append(out, wire.RepoRiskToProto(r))
	}
	return connect.NewResponse(&daemonv1.ProjectRiskResponse{Repos: out}), nil
}

func (s *projectServer) RecycleProject(ctx context.Context, req *connect.Request[daemonv1.RecycleProjectRequest]) (*connect.Response[daemonv1.RecycleProjectResponse], error) {
	if err := s.api.RecycleProject(ctx, req.Msg.GetRef()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.RecycleProjectResponse{}), nil
}

func (s *projectServer) DeleteProject(ctx context.Context, req *connect.Request[daemonv1.DeleteProjectRequest]) (*connect.Response[daemonv1.DeleteProjectResponse], error) {
	if err := s.api.DeleteProject(ctx, req.Msg.GetRef()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.DeleteProjectResponse{}), nil
}

func (s *projectServer) OpenProject(ctx context.Context, req *connect.Request[daemonv1.OpenProjectRequest]) (*connect.Response[daemonv1.OpenProjectResponse], error) {
	if err := s.api.OpenProject(ctx, req.Msg.GetRef()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.OpenProjectResponse{}), nil
}

func (s *projectServer) ProjectOf(ctx context.Context, req *connect.Request[daemonv1.ProjectOfRequest]) (*connect.Response[daemonv1.ProjectOfResponse], error) {
	slug, found, err := s.api.ProjectOf(ctx, req.Msg.GetSessionId())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.ProjectOfResponse{Slug: slug, Found: found}), nil
}
