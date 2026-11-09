package daemonclient

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/hive/projectapi"
)

// ErrProjectsUnsupported is a daemon whose hive predates project sessions.
var ErrProjectsUnsupported = errors.New("the server's hive is too old for project sessions; install the same hive version on both machines")

var _ projectapi.API = (*Client)(nil)

func projectError(err error) error {
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		return ErrProjectsUnsupported
	}
	return wire.FromConnectError(err)
}

func (c *Client) ListProjects(ctx context.Context) ([]projectapi.Project, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.ListProjectsResponse], error) {
		return c.projects.ListProjects(ctx, connect.NewRequest(&daemonv1.ListProjectsRequest{}))
	})
	if err != nil {
		return nil, projectError(err)
	}
	out := make([]projectapi.Project, 0, len(resp.Msg.GetProjects()))
	for _, p := range resp.Msg.GetProjects() {
		out = append(out, wire.ProjectFromProto(p))
	}
	return out, nil
}

func (c *Client) ProjectDefinitions(ctx context.Context) ([]projectapi.Definition, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.ProjectDefinitionsResponse], error) {
		return c.projects.ProjectDefinitions(ctx, connect.NewRequest(&daemonv1.ProjectDefinitionsRequest{}))
	})
	if err != nil {
		return nil, projectError(err)
	}
	out := make([]projectapi.Definition, 0, len(resp.Msg.GetDefinitions()))
	for _, d := range resp.Msg.GetDefinitions() {
		out = append(out, wire.DefinitionFromProto(d))
	}
	return out, nil
}

func (c *Client) DefaultCloneStrategy(ctx context.Context, remote string) (string, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.DefaultCloneStrategyResponse], error) {
		return c.projects.DefaultCloneStrategy(ctx, connect.NewRequest(&daemonv1.DefaultCloneStrategyRequest{Remote: remote}))
	})
	if err != nil {
		return "", projectError(err)
	}
	return resp.Msg.GetCloneStrategy(), nil
}

func (c *Client) CreateProject(ctx context.Context, opts projectsvc.CreateOptions) (projectapi.Project, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return projectapi.Project{}, err
	}
	began := c.now()
	stream, err := c.projects.CreateProject(ctx, connect.NewRequest(wire.CreateProjectOptionsToProto(opts)))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnimplemented {
			return projectapi.Project{}, ErrProjectsUnsupported
		}
		return projectapi.Project{}, c.streamFailure(err, daemonv1connect.ProjectServiceCreateProjectProcedure, began)
	}
	defer func() { _ = stream.Close() }()

	var created *projectapi.Project
	for stream.Receive() {
		switch event := stream.Msg().GetEvent().(type) {
		case *daemonv1.CreateProjectResponse_Step:
			if opts.OnStep != nil {
				opts.OnStep(wire.StepFromProto(event.Step))
			}
		case *daemonv1.CreateProjectResponse_Project:
			p := wire.ProjectFromProto(event.Project)
			created = &p
		}
	}
	if err := stream.Err(); err != nil {
		if connect.CodeOf(err) == connect.CodeUnimplemented {
			return projectapi.Project{}, ErrProjectsUnsupported
		}
		return projectapi.Project{}, c.streamFailure(err, daemonv1connect.ProjectServiceCreateProjectProcedure, began)
	}
	if created == nil {
		return projectapi.Project{}, fmt.Errorf("create project: the daemon ended the stream without a project")
	}
	return *created, nil
}

func (c *Client) ProjectRisk(ctx context.Context, ref string) ([]projectapi.RepoRisk, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return nil, err
	}
	resp, err := retryRead(func() (*connect.Response[daemonv1.ProjectRiskResponse], error) {
		return c.projects.ProjectRisk(ctx, connect.NewRequest(&daemonv1.ProjectRiskRequest{Ref: ref}))
	})
	if err != nil {
		return nil, projectError(err)
	}
	out := make([]projectapi.RepoRisk, 0, len(resp.Msg.GetRepos()))
	for _, r := range resp.Msg.GetRepos() {
		out = append(out, wire.RepoRiskFromProto(r))
	}
	return out, nil
}

func (c *Client) RecycleProject(ctx context.Context, ref string) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.projects.RecycleProject(ctx, connect.NewRequest(&daemonv1.RecycleProjectRequest{Ref: ref}))
	if err != nil {
		return projectError(err)
	}
	return nil
}

func (c *Client) DeleteProject(ctx context.Context, ref string) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.projects.DeleteProject(ctx, connect.NewRequest(&daemonv1.DeleteProjectRequest{Ref: ref}))
	if err != nil {
		return projectError(err)
	}
	return nil
}

// OpenProject is not retried: a second start after a lost answer could
// spawn the agent twice.
func (c *Client) OpenProject(ctx context.Context, ref string) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.projects.OpenProject(ctx, connect.NewRequest(&daemonv1.OpenProjectRequest{Ref: ref}))
	if err != nil {
		return projectError(err)
	}
	return nil
}

func (c *Client) ProjectOf(ctx context.Context, sessionID string) (string, bool, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.ProjectOfResponse], error) {
		return c.projects.ProjectOf(ctx, connect.NewRequest(&daemonv1.ProjectOfRequest{SessionId: sessionID}))
	})
	if err != nil {
		return "", false, projectError(err)
	}
	return resp.Msg.GetSlug(), resp.Msg.GetFound(), nil
}
