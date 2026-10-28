// Package wire converts between the hive.daemon.v1 messages and the Go types
// sessionapi.API speaks, for both the daemon and its client.
package wire

import (
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/status"
)

func SessionToProto(s session.Session) *daemonv1.Session {
	return &daemonv1.Session{
		Id:            s.ID,
		Name:          s.Name,
		Slug:          s.Slug,
		Path:          s.Path,
		Remote:        s.Remote,
		State:         string(s.State),
		CloneStrategy: s.CloneStrategy,
		Tags:          s.Tags,
		Metadata:      s.Metadata,
		CreatedAt:     timestampToProto(s.CreatedAt),
		UpdatedAt:     timestampToProto(s.UpdatedAt),
	}
}

// SessionFromProto reads timestamps back in local time, as the store does, so
// a session compares equal however it was read.
func SessionFromProto(p *daemonv1.Session) session.Session {
	return session.Session{
		ID:            p.GetId(),
		Name:          p.GetName(),
		Slug:          p.GetSlug(),
		Path:          p.GetPath(),
		Remote:        p.GetRemote(),
		State:         session.State(p.GetState()),
		CloneStrategy: p.GetCloneStrategy(),
		Tags:          p.GetTags(),
		Metadata:      p.GetMetadata(),
		CreatedAt:     timestampFromProto(p.GetCreatedAt()),
		UpdatedAt:     timestampFromProto(p.GetUpdatedAt()),
	}
}

func timestampToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func timestampFromProto(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime().Local()
}

func CreateOptionsToProto(o sessionsvc.CreateOptions) *daemonv1.CreateRequest {
	return &daemonv1.CreateRequest{
		Name:            o.Name,
		SessionId:       o.SessionID,
		Prompt:          o.Prompt,
		Remote:          o.Remote,
		Source:          o.Source,
		UseBatchSpawn:   o.UseBatchSpawn,
		CloneStrategy:   o.CloneStrategy,
		SkipSpawn:       o.SkipSpawn,
		AgentKey:        o.AgentKey,
		Tags:            o.Tags,
		Env:             o.Env,
		CollisionSuffix: o.CollisionSuffix,
	}
}

func CreateOptionsFromProto(p *daemonv1.CreateRequest) sessionsvc.CreateOptions {
	return sessionsvc.CreateOptions{
		Name:            p.GetName(),
		SessionID:       p.GetSessionId(),
		Prompt:          p.GetPrompt(),
		Remote:          p.GetRemote(),
		Source:          p.GetSource(),
		UseBatchSpawn:   p.GetUseBatchSpawn(),
		CloneStrategy:   p.GetCloneStrategy(),
		SkipSpawn:       p.GetSkipSpawn(),
		AgentKey:        p.GetAgentKey(),
		Tags:            p.GetTags(),
		Env:             p.GetEnv(),
		CollisionSuffix: p.GetCollisionSuffix(),
	}
}

func LaunchRequestToProto(r sessionsvc.LaunchRequest) *daemonv1.CreateFromRequestRequest {
	return &daemonv1.CreateFromRequestRequest{
		Name:            r.Name,
		Prompt:          r.Prompt,
		Agent:           r.Agent,
		Repo:            r.Repo,
		CollisionSuffix: r.CollisionSuffix,
		Tags:            r.Tags,
		Env:             r.Env,
	}
}

func LaunchRequestFromProto(p *daemonv1.CreateFromRequestRequest) sessionsvc.LaunchRequest {
	return sessionsvc.LaunchRequest{
		Name:            p.GetName(),
		Prompt:          p.GetPrompt(),
		Agent:           p.GetAgent(),
		Repo:            p.GetRepo(),
		CollisionSuffix: p.GetCollisionSuffix(),
		Tags:            p.GetTags(),
		Env:             p.GetEnv(),
	}
}

func WindowsToProto(windows []multiplexer.WindowSpec) []*daemonv1.WindowSpec {
	out := make([]*daemonv1.WindowSpec, 0, len(windows))
	for _, w := range windows {
		panes := make([]*daemonv1.PaneSpec, 0, len(w.Panes))
		for _, p := range w.Panes {
			panes = append(panes, &daemonv1.PaneSpec{
				Command:          p.Command,
				WorkingDirectory: p.WorkingDirectory,
				Size:             p.Size,
				Split:            string(p.Split),
			})
		}
		out = append(out, &daemonv1.WindowSpec{
			Name:             w.Name,
			Command:          w.Command,
			WorkingDirectory: w.WorkingDirectory,
			Focus:            w.Focus,
			Panes:            panes,
		})
	}
	return out
}

func WindowsFromProto(windows []*daemonv1.WindowSpec) []multiplexer.WindowSpec {
	out := make([]multiplexer.WindowSpec, 0, len(windows))
	for _, w := range windows {
		var panes []multiplexer.PaneSpec
		for _, p := range w.GetPanes() {
			panes = append(panes, multiplexer.PaneSpec{
				Command:          p.GetCommand(),
				WorkingDirectory: p.GetWorkingDirectory(),
				Size:             p.GetSize(),
				Split:            multiplexer.SplitDirection(p.GetSplit()),
			})
		}
		out = append(out, multiplexer.WindowSpec{
			Name:             w.GetName(),
			Command:          w.GetCommand(),
			WorkingDirectory: w.GetWorkingDirectory(),
			Focus:            w.GetFocus(),
			Panes:            panes,
		})
	}
	return out
}

func LaunchOptionsToProto(o sessionsvc.LaunchOptions) *daemonv1.LaunchOptionsResponse {
	repos := make([]*daemonv1.LaunchRepository, 0, len(o.Repositories))
	for _, r := range o.Repositories {
		repos = append(repos, &daemonv1.LaunchRepository{Name: r.Name, Remote: r.Remote, Source: r.Source})
	}
	return &daemonv1.LaunchOptionsResponse{
		Repositories:      repos,
		DefaultRepository: o.DefaultRepository,
		Agents:            o.Agents,
		DefaultAgent:      o.DefaultAgent,
	}
}

func LaunchOptionsFromProto(p *daemonv1.LaunchOptionsResponse) sessionsvc.LaunchOptions {
	var repos []sessionsvc.LaunchRepository
	for _, r := range p.GetRepositories() {
		repos = append(repos, sessionsvc.LaunchRepository{Name: r.GetName(), Remote: r.GetRemote(), Source: r.GetSource()})
	}
	return sessionsvc.LaunchOptions{
		Repositories:      repos,
		DefaultRepository: p.GetDefaultRepository(),
		Agents:            p.GetAgents(),
		DefaultAgent:      p.GetDefaultAgent(),
	}
}

func RootsToProto(roots []status.RootRepoTarget) []*daemonv1.RootRepoTarget {
	out := make([]*daemonv1.RootRepoTarget, 0, len(roots))
	for _, r := range roots {
		out = append(out, &daemonv1.RootRepoTarget{Name: r.Name, Path: r.Path})
	}
	return out
}

func RootsFromProto(roots []*daemonv1.RootRepoTarget) []status.RootRepoTarget {
	out := make([]status.RootRepoTarget, 0, len(roots))
	for _, r := range roots {
		out = append(out, status.RootRepoTarget{Name: r.GetName(), Path: r.GetPath()})
	}
	return out
}

func StatusesToProto(statuses map[string]status.TerminalStatus) map[string]*daemonv1.TerminalStatus {
	out := make(map[string]*daemonv1.TerminalStatus, len(statuses))
	for key, s := range statuses {
		out[key] = terminalStatusToProto(s)
	}
	return out
}

func StatusesFromProto(statuses map[string]*daemonv1.TerminalStatus) map[string]status.TerminalStatus {
	out := make(map[string]status.TerminalStatus, len(statuses))
	for key, s := range statuses {
		out[key] = terminalStatusFromProto(s)
	}
	return out
}

func terminalStatusToProto(s status.TerminalStatus) *daemonv1.TerminalStatus {
	windows := make([]*daemonv1.WindowStatus, 0, len(s.Windows))
	for _, w := range s.Windows {
		panes := make([]*daemonv1.PaneStatus, 0, len(w.Panes))
		for _, p := range w.Panes {
			panes = append(panes, &daemonv1.PaneStatus{
				PaneId: p.PaneID, Status: string(p.Status), Tool: p.Tool, PaneContent: p.PaneContent, IsAgent: p.IsAgent,
			})
		}
		windows = append(windows, &daemonv1.WindowStatus{
			WindowIndex: w.WindowIndex, WindowName: w.WindowName, Status: string(w.Status),
			Tool: w.Tool, PaneContent: w.PaneContent, Panes: panes,
		})
	}
	var errText string
	if s.Error != nil {
		errText = s.Error.Error()
	}
	return &daemonv1.TerminalStatus{
		SessionPresent: s.SessionPresent,
		PresenceKnown:  s.PresenceKnown,
		Status:         string(s.Status),
		Tool:           s.Tool,
		WindowName:     s.WindowName,
		PaneContent:    s.PaneContent,
		IsLoading:      s.IsLoading,
		Error:          errText,
		Windows:        windows,
	}
}

func terminalStatusFromProto(p *daemonv1.TerminalStatus) status.TerminalStatus {
	var windows []status.WindowStatus
	for _, w := range p.GetWindows() {
		var panes []status.PaneStatus
		for _, pane := range w.GetPanes() {
			panes = append(panes, status.PaneStatus{
				PaneID: pane.GetPaneId(), Status: terminal.Status(pane.GetStatus()), Tool: pane.GetTool(),
				PaneContent: pane.GetPaneContent(), IsAgent: pane.GetIsAgent(),
			})
		}
		windows = append(windows, status.WindowStatus{
			WindowIndex: w.GetWindowIndex(), WindowName: w.GetWindowName(), Status: terminal.Status(w.GetStatus()),
			Tool: w.GetTool(), PaneContent: w.GetPaneContent(), Panes: panes,
		})
	}
	var err error
	if text := p.GetError(); text != "" {
		err = errors.New(text)
	}
	return status.TerminalStatus{
		SessionPresent: p.GetSessionPresent(),
		PresenceKnown:  p.GetPresenceKnown(),
		Status:         terminal.Status(p.GetStatus()),
		Tool:           p.GetTool(),
		WindowName:     p.GetWindowName(),
		PaneContent:    p.GetPaneContent(),
		IsLoading:      p.GetIsLoading(),
		Error:          err,
		Windows:        windows,
	}
}
