package wire

import (
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/internal/hive/projectapi"
)

func ProjectToProto(p projectapi.Project) *daemonv1.Project {
	repos := make([]*daemonv1.ProjectRepo, 0, len(p.Repos))
	for _, r := range p.Repos {
		repos = append(repos, &daemonv1.ProjectRepo{
			Dir: r.Dir, SessionId: r.SessionID, Remote: r.Remote, Path: r.Path,
			Branch: r.Branch, CloneStrategy: r.CloneStrategy, Missing: r.Missing,
		})
	}
	return &daemonv1.Project{
		Id: p.ID, Name: p.Name, Slug: p.Slug, Definition: p.Definition, Root: p.Root,
		Repos: repos, CreatedAt: timestampToProto(p.CreatedAt),
	}
}

func ProjectFromProto(p *daemonv1.Project) projectapi.Project {
	repos := make([]projectapi.Repo, 0, len(p.GetRepos()))
	for _, r := range p.GetRepos() {
		repos = append(repos, projectapi.Repo{
			Dir: r.GetDir(), SessionID: r.GetSessionId(), Remote: r.GetRemote(), Path: r.GetPath(),
			Branch: r.GetBranch(), CloneStrategy: r.GetCloneStrategy(), Missing: r.GetMissing(),
		})
	}
	return projectapi.Project{
		ID: p.GetId(), Name: p.GetName(), Slug: p.GetSlug(), Definition: p.GetDefinition(), Root: p.GetRoot(),
		Repos: repos, CreatedAt: timestampFromProto(p.GetCreatedAt()),
	}
}

func RepoSpecsToProto(specs []projectsvc.RepoSpec) []*daemonv1.ProjectRepoSpec {
	out := make([]*daemonv1.ProjectRepoSpec, 0, len(specs))
	for _, s := range specs {
		out = append(out, &daemonv1.ProjectRepoSpec{Remote: s.Remote, Dir: s.Dir, CloneStrategy: s.CloneStrategy})
	}
	return out
}

func RepoSpecsFromProto(specs []*daemonv1.ProjectRepoSpec) []projectsvc.RepoSpec {
	out := make([]projectsvc.RepoSpec, 0, len(specs))
	for _, s := range specs {
		out = append(out, projectsvc.RepoSpec{Remote: s.GetRemote(), Dir: s.GetDir(), CloneStrategy: s.GetCloneStrategy()})
	}
	return out
}

func DefinitionToProto(d projectapi.Definition) *daemonv1.ProjectDefinition {
	return &daemonv1.ProjectDefinition{Name: d.Name, Agent: d.Agent, Repos: RepoSpecsToProto(d.Repos)}
}

func DefinitionFromProto(d *daemonv1.ProjectDefinition) projectapi.Definition {
	return projectapi.Definition{Name: d.GetName(), Agent: d.GetAgent(), Repos: RepoSpecsFromProto(d.GetRepos())}
}

// CreateProjectOptionsToProto drops OnStep and Progress: the stream carries
// the steps back.
func CreateProjectOptionsToProto(o projectsvc.CreateOptions) *daemonv1.CreateProjectRequest {
	return &daemonv1.CreateProjectRequest{
		Name: o.Name, Definition: o.Project, Repos: RepoSpecsToProto(o.Repos), Prompt: o.Prompt, Agent: o.Agent,
	}
}

func CreateProjectOptionsFromProto(r *daemonv1.CreateProjectRequest) projectsvc.CreateOptions {
	return projectsvc.CreateOptions{
		Name: r.GetName(), Project: r.GetDefinition(), Repos: RepoSpecsFromProto(r.GetRepos()),
		Prompt: r.GetPrompt(), Agent: r.GetAgent(),
	}
}

func StepToProto(s projectsvc.Step) *daemonv1.ProjectStep {
	return &daemonv1.ProjectStep{Dir: s.Dir, Remote: s.Remote, State: string(s.State), Detail: s.Detail}
}

func StepFromProto(s *daemonv1.ProjectStep) projectsvc.Step {
	return projectsvc.Step{Dir: s.GetDir(), Remote: s.GetRemote(), State: projectsvc.StepState(s.GetState()), Detail: s.GetDetail()}
}

func RepoRiskToProto(r projectapi.RepoRisk) *daemonv1.ProjectRepoRisk {
	return &daemonv1.ProjectRepoRisk{
		Dir: r.Dir, CloneStrategy: r.CloneStrategy, Missing: r.Missing,
		UncommittedChanges: r.UncommittedChanges, UnpushedCommits: r.UnpushedCommits,
	}
}

func RepoRiskFromProto(r *daemonv1.ProjectRepoRisk) projectapi.RepoRisk {
	return projectapi.RepoRisk{
		Dir: r.GetDir(), CloneStrategy: r.GetCloneStrategy(), Missing: r.GetMissing(),
		UncommittedChanges: r.GetUncommittedChanges(), UnpushedCommits: r.GetUnpushedCommits(),
	}
}
