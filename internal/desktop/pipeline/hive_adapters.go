package pipeline

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
)

type SessionCreator interface {
	CreateSession(context.Context, hive.CreateOptions) (*session.Session, error)
}

type sessionLaunchOptionsSource interface {
	SessionLaunchOptions(context.Context) (hive.SessionLaunchOptions, error)
	ResolveSessionLaunchRepository(context.Context, string) (hive.SessionLaunchRepository, error)
}

type HiveSessionLauncher struct{ sessions SessionCreator }

func NewHiveSessionLauncher(sessions SessionCreator) *HiveSessionLauncher {
	return &HiveSessionLauncher{sessions: sessions}
}

func (l *HiveSessionLauncher) LaunchSession(ctx context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error) {
	if l.sessions == nil {
		return SessionExecutionOutcome{}, fmt.Errorf("launch session: hive session service is unavailable")
	}
	remote, source := req.Repo, ""
	if known, ok := l.sessions.(sessionLaunchOptionsSource); ok {
		repo, err := known.ResolveSessionLaunchRepository(ctx, req.Repo)
		if err != nil {
			return SessionExecutionOutcome{}, fmt.Errorf("resolve launch repository: %w", err)
		}
		remote, source = repo.Remote, repo.Source
	}
	s, err := l.sessions.CreateSession(ctx, hive.CreateOptions{Name: req.Name, Prompt: req.Prompt, Remote: remote, Source: source, AgentKey: req.Agent, Background: true, UseBatchSpawn: false})
	if err != nil {
		return SessionExecutionOutcome{}, fmt.Errorf("create hive session: %w", err)
	}
	return SessionExecutionOutcome{ID: s.ID, Name: s.Name}, nil
}

// SessionLaunchOptions exposes only labels, remotes, and configured agent keys
// to the desktop; local source paths remain in the Hive service.
func (l *HiveSessionLauncher) SessionLaunchOptions(ctx context.Context) (SessionLaunchOptions, error) {
	known, ok := l.sessions.(sessionLaunchOptionsSource)
	if !ok {
		return SessionLaunchOptions{}, fmt.Errorf("session launch options are unavailable")
	}
	options, err := known.SessionLaunchOptions(ctx)
	if err != nil {
		return SessionLaunchOptions{}, err
	}
	view := SessionLaunchOptions{
		DefaultRepository: options.DefaultRepository,
		Agents:            options.Agents,
		DefaultAgent:      options.DefaultAgent,
	}
	for _, repo := range options.Repositories {
		view.Repositories = append(view.Repositories, SessionLaunchRepository{Name: repo.Name, Repository: repo.Remote})
	}
	return view, nil
}

type DurableMessageService interface {
	Publish(context.Context, messaging.Message, []string) (messaging.PublishResult, error)
}
type HiveMessagePublisher struct{ messages DurableMessageService }

func NewHiveMessagePublisher(messages DurableMessageService) *HiveMessagePublisher {
	return &HiveMessagePublisher{messages: messages}
}

func (p *HiveMessagePublisher) PublishMessage(ctx context.Context, payload, topic string) (string, error) {
	if p.messages == nil {
		return "", fmt.Errorf("publish message: hive message service is unavailable")
	}
	result, err := p.messages.Publish(ctx, messaging.Message{Payload: payload, Sender: "hive-desktop", SessionID: ""}, []string{topic})
	if err != nil {
		return "", fmt.Errorf("publish message: %w", err)
	}
	if len(result.Topics) != 1 || result.Topics[0] != topic {
		return "", fmt.Errorf("publish message: expected exact topic %q, got %v", topic, result.Topics)
	}
	return result.Topics[0], nil
}
