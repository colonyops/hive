package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/prompt"
)

const OrchestratorTag = "orchestrator"

// MaxOrchestratorWait stays under Claude Code's five-minute silent-call limit.
const MaxOrchestratorWait = 4 * time.Minute

const (
	// Under sixty seconds: some clients time out there, after messages were acked.
	defaultOrchestratorWait = 50 * time.Second
	orchestratorMessagePoll = time.Second
	defaultStatusPoll       = 2 * time.Second
)

type orchestrationLauncher interface {
	LaunchSession(ctx context.Context, req dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error)
}

// orchestrationSessions is what SessionsService already answers for the UI.
type orchestrationSessions interface {
	SessionLaunchOptions(ctx context.Context) (dispatch.SessionLaunchOptions, error)
	SessionStatuses(ctx context.Context) (SessionStatusSnapshot, error)
}

type hiveSessions interface {
	ListSessions(ctx context.Context) ([]session.Session, error)
	GetSession(ctx context.Context, id string) (session.Session, error)
}

type promptDriver interface {
	Peek(ctx context.Context, sess session.Session, lines int) (prompt.SessionPeek, error)
	Send(ctx context.Context, sess session.Session, text string, opts prompt.SendOptions) (prompt.SessionPeek, error)
	SendKeys(ctx context.Context, sess session.Session, keys []string, lines int) (prompt.SessionPeek, error)
}

type messageBus interface {
	Publish(ctx context.Context, msg messaging.Message, topics []string) (messaging.PublishResult, error)
	GetUnread(ctx context.Context, consumerID, topic string) ([]messaging.Message, error)
	Acknowledge(ctx context.Context, consumerID string, messageIDs []string) error
}

type orchestratorAuthorizer interface {
	AuthorizeOrchestrator(ctx context.Context, token string) (OrchestratorCaller, error)
}

type orchestratorTokenStore interface {
	Create(ctx context.Context, name, hash, hint string) (stores.OrchestratorToken, error)
	List(ctx context.Context) ([]stores.OrchestratorToken, error)
	GetByHash(ctx context.Context, hash string) (stores.OrchestratorToken, error)
	Touch(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) (bool, error)
}

// OrchestrationService drives hive sessions for the hive-orchestrator MCP server.
type OrchestrationService struct {
	launcher orchestrationLauncher
	ui       orchestrationSessions
	hive     func() hiveSessions
	prompts  promptDriver
	messages func() messageBus
	auth     orchestratorAuthorizer
	tokens   orchestratorTokenStore
	mcpBase  interface{ MCPBaseURL(context.Context) string }
	done     <-chan struct{}
	now      func() time.Time
	logger   zerolog.Logger
	// claimLock is a channel so a waiter can give up on ctx.
	claimLock chan struct{}
}

type OrchestrationDeps struct {
	Launcher orchestrationLauncher
	Sessions orchestrationSessions
	// Hive and Messages are getters because the engine rebuilds its services
	// on a config reload.
	Hive     func() hiveSessions
	Prompts  promptDriver
	Messages func() messageBus
	Auth     orchestratorAuthorizer
	Tokens   orchestratorTokenStore
	MCPBase  interface{ MCPBaseURL(context.Context) string }
	Done     <-chan struct{}
	Logger   zerolog.Logger
}

func newOrchestrationService(d OrchestrationDeps) *OrchestrationService {
	return &OrchestrationService{
		launcher: d.Launcher, ui: d.Sessions, hive: d.Hive, prompts: d.Prompts, messages: d.Messages,
		auth: d.Auth, tokens: d.Tokens, mcpBase: d.MCPBase,
		done: d.Done, now: time.Now, logger: d.Logger,
		claimLock: make(chan struct{}, 1),
	}
}

// Authorize accepts a Settings access token or a chat's session token.
func (s *OrchestrationService) Authorize(ctx context.Context, token string) (OrchestratorCaller, error) {
	if !strings.HasPrefix(token, orchestratorTokenPrefix) {
		return s.auth.AuthorizeOrchestrator(ctx, token)
	}
	rec, err := s.tokens.GetByHash(ctx, hashToken(token))
	if stores.IsNotFound(err) {
		return OrchestratorCaller{}, Errorf(KindUnauthenticated, "the access token is unknown or revoked")
	}
	if err != nil {
		return OrchestratorCaller{}, Wrap(err, KindInternal, "looking up an access token")
	}
	if err := s.tokens.Touch(ctx, rec.ID); err != nil {
		s.logger.Warn().Ctx(ctx).Err(err).Int64("token", rec.ID).Msg("recording access token use")
	}
	return OrchestratorCaller{Token: rec.ID, Name: rec.Name, authorized: true}, nil
}

func (c OrchestratorCaller) check() error {
	if !c.authorized {
		return Errorf(KindUnauthenticated, "the caller is not authorized")
	}
	return nil
}

// consumer is per workspace so a new chat does not re-take acked messages.
func (c OrchestratorCaller) consumer() string {
	if c.Workspace == "" {
		return fmt.Sprintf("token.%d", c.Token)
	}
	return "agentws." + c.Workspace
}

const orchestratorTokenPrefix = "hvo_"

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type CreatedOrchestratorToken struct {
	stores.OrchestratorToken
	Token string `json:"token"`
}

func (s *OrchestrationService) CreateToken(ctx context.Context, name string) (CreatedOrchestratorToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreatedOrchestratorToken{}, Errorf(KindInvalid, "a token name is required")
	}
	existing, err := s.tokens.List(ctx)
	if err != nil {
		return CreatedOrchestratorToken{}, Wrap(err, KindInternal, "listing access tokens")
	}
	for _, t := range existing {
		if strings.EqualFold(t.Name, name) {
			return CreatedOrchestratorToken{}, Errorf(KindConflict, "a token named %q already exists", name)
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return CreatedOrchestratorToken{}, Wrap(err, KindInternal, "generating an access token")
	}
	token := orchestratorTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	rec, err := s.tokens.Create(ctx, name, hashToken(token), token[len(token)-4:])
	if err != nil {
		return CreatedOrchestratorToken{}, Wrap(err, KindInternal, "saving access token %q", name)
	}
	return CreatedOrchestratorToken{OrchestratorToken: rec, Token: token}, nil
}

func (s *OrchestrationService) Tokens(ctx context.Context) ([]stores.OrchestratorToken, error) {
	list, err := s.tokens.List(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing access tokens")
	}
	return list, nil
}

func (s *OrchestrationService) ServerURL(ctx context.Context) string {
	base := s.mcpBase.MCPBaseURL(ctx)
	descriptor, ok := mcpcatalog.Lookup(mcpcatalog.Orchestrator)
	if base == "" || !ok {
		return ""
	}
	return base + descriptor.RuntimePath
}

func (s *OrchestrationService) RevokeToken(ctx context.Context, id int64) error {
	removed, err := s.tokens.Delete(ctx, id)
	if err != nil {
		return Wrap(err, KindInternal, "revoking access token %d", id)
	}
	if !removed {
		return Errorf(KindNotFound, "no access token %d", id)
	}
	return nil
}

func (s *OrchestrationService) Repositories(ctx context.Context, caller OrchestratorCaller) (dispatch.SessionLaunchOptions, error) {
	if err := caller.check(); err != nil {
		return dispatch.SessionLaunchOptions{}, err
	}
	opts, err := s.ui.SessionLaunchOptions(ctx)
	if err != nil {
		return dispatch.SessionLaunchOptions{}, Wrap(err, KindUnavailable, "reading the repositories hive knows")
	}
	return opts, nil
}

type OrchestratedSessionRequest struct {
	Repository string
	Name       string
	Prompt     string
	Agent      string
	Tags       []string
}

func (s *OrchestrationService) StartSession(ctx context.Context, caller OrchestratorCaller, req OrchestratedSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	if err := caller.check(); err != nil {
		return dispatch.SessionExecutionOutcome{}, err
	}
	name := strings.TrimSpace(req.Name)
	repo, err := s.resolveRepository(ctx, req.Repository)
	if err != nil {
		return dispatch.SessionExecutionOutcome{}, err
	}
	if name == "" {
		return dispatch.SessionExecutionOutcome{}, Errorf(KindInvalid, "session name is required")
	}
	if err := session.ValidateName(name); err != nil {
		return dispatch.SessionExecutionOutcome{}, Wrap(err, KindInvalid, "session name")
	}
	tags := []string{OrchestratorTag}
	for _, tag := range req.Tags {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	s.logger.Info().Ctx(ctx).Str("workspace", caller.Workspace).Str("session_name", name).Str("repository", repo).Msg("orchestrator starting a session")
	out, err := s.launcher.LaunchSession(ctx, dispatch.LaunchSessionRequest{
		Name: name, Prompt: strings.TrimSpace(req.Prompt), Agent: strings.TrimSpace(req.Agent), Repo: repo, Tags: tags,
	})
	if errors.Is(err, session.ErrDuplicateName) {
		return dispatch.SessionExecutionOutcome{}, Errorf(KindConflict, "a session named %q already exists", name)
	}
	if err != nil {
		return dispatch.SessionExecutionOutcome{}, Wrap(err, KindInternal, "starting session %q in %s", name, repo)
	}
	return out, nil
}

func (s *OrchestrationService) resolveRepository(ctx context.Context, repository string) (string, error) {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return "", Errorf(KindInvalid, "repository is required")
	}
	if strings.Contains(repository, "/") || strings.Contains(repository, ":") {
		return repository, nil
	}
	opts, err := s.ui.SessionLaunchOptions(ctx)
	if err != nil {
		return "", Wrap(err, KindUnavailable, "reading the repositories hive knows")
	}
	for _, r := range opts.Repositories {
		if strings.EqualFold(r.Name, repository) {
			return r.Repository, nil
		}
	}
	return "", Errorf(KindNotFound, "no repository named %q; list_repositories shows the known ones, or pass a remote URL", repository)
}

type OrchestratedSession struct {
	session.Session
	Running     bool
	AgentStatus string
}

// Sessions lists active orchestrator-tagged sessions unless all is set.
func (s *OrchestrationService) Sessions(ctx context.Context, caller OrchestratorCaller, all bool, tags []string) ([]OrchestratedSession, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	out, _, err := s.sessions(ctx, all, tags, false)
	return out, err
}

var errStatusUnread = errors.New("agent statuses could not be read")

// strictStatus fails on a status error so a wait does not read it as missing.
func (s *OrchestrationService) sessions(ctx context.Context, all bool, tags []string, strictStatus bool) ([]OrchestratedSession, time.Duration, error) {
	sessions, err := s.hive().ListSessions(ctx)
	if err != nil {
		return nil, 0, Wrap(err, KindInternal, "listing hive sessions")
	}
	slices.SortStableFunc(sessions, func(a, b session.Session) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if !all {
		tags = append([]string{OrchestratorTag}, tags...)
	}
	interval := defaultStatusPoll
	statuses := map[string]SessionStatus{}
	if snap, err := s.ui.SessionStatuses(ctx); err != nil {
		if strictStatus {
			return nil, 0, fmt.Errorf("%w: %w", errStatusUnread, err)
		}
		s.logger.Warn().Ctx(ctx).Err(err).Msg("orchestrator session list without agent statuses")
	} else {
		if snap.PollIntervalMS > 0 {
			interval = time.Duration(snap.PollIntervalMS) * time.Millisecond
		}
		for _, st := range snap.Items {
			statuses[st.SessionID] = st
		}
	}
	out := []OrchestratedSession{}
	for _, sess := range sessions {
		if !all && sess.State != session.StateActive {
			continue
		}
		if !hasEveryTag(sess.Tags, tags) {
			continue
		}
		item := OrchestratedSession{Session: sess}
		if st, ok := statuses[sess.ID]; ok {
			item.Running = st.Running
			item.AgentStatus = mostUrgentStatus(st.Windows)
		}
		out = append(out, item)
	}
	return out, interval, nil
}

func mostUrgentStatus(windows []SessionWindowStatus) string {
	rank := map[string]int{"approval": 4, "question": 4, "active": 3, "ready": 2, "missing": 1}
	best := ""
	for _, w := range windows {
		if rank[w.Status] > rank[best] {
			best = w.Status
		}
	}
	return best
}

func hasEveryTag(have, want []string) bool {
	for _, tag := range want {
		if !slices.Contains(have, tag) {
			return false
		}
	}
	return true
}

type PromptSend struct {
	SessionID string
	Text      string
	NoSubmit  bool
	Lines     int
}

func (s *OrchestrationService) Peek(ctx context.Context, caller OrchestratorCaller, id string, lines int) (prompt.SessionPeek, error) {
	if err := caller.check(); err != nil {
		return prompt.SessionPeek{}, err
	}
	sess, err := s.hive().GetSession(ctx, id)
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, id)
	}
	peek, err := s.prompts.Peek(ctx, sess, lines)
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, id)
	}
	return peek, nil
}

func (s *OrchestrationService) SendPrompt(ctx context.Context, caller OrchestratorCaller, req PromptSend) (prompt.SessionPeek, error) {
	if err := caller.check(); err != nil {
		return prompt.SessionPeek{}, err
	}
	if strings.TrimSpace(req.Text) == "" {
		return prompt.SessionPeek{}, Errorf(KindInvalid, "text is required")
	}
	sess, err := s.hive().GetSession(ctx, req.SessionID)
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, req.SessionID)
	}
	peek, err := s.prompts.Send(ctx, sess, req.Text, prompt.SendOptions{NoSubmit: req.NoSubmit, Lines: req.Lines})
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, req.SessionID)
	}
	return peek, nil
}

func (s *OrchestrationService) SendKeys(ctx context.Context, caller OrchestratorCaller, id string, keys []string, lines int) (prompt.SessionPeek, error) {
	if err := caller.check(); err != nil {
		return prompt.SessionPeek{}, err
	}
	if len(keys) == 0 {
		return prompt.SessionPeek{}, Errorf(KindInvalid, "at least one key is required")
	}
	sess, err := s.hive().GetSession(ctx, id)
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, id)
	}
	peek, err := s.prompts.SendKeys(ctx, sess, keys, lines)
	if err != nil {
		return prompt.SessionPeek{}, driveError(err, id)
	}
	return peek, nil
}

func driveError(err error, id string) error {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return Errorf(KindNotFound, "no hive session %q", id)
	case errors.Is(err, prompt.ErrAgentPaneNotFound):
		return Wrap(err, KindNotFound, "session %q has no agent pane; its tmux session may not be running", id)
	default:
		return Wrap(err, KindInternal, "driving session %q", id)
	}
}

func (s *OrchestrationService) Publish(ctx context.Context, caller OrchestratorCaller, topic, payload string) ([]string, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, Errorf(KindInvalid, "topic is required")
	}
	result, err := s.messages().Publish(ctx, messaging.Message{Payload: payload, Sender: caller.consumer()}, []string{topic})
	if err != nil {
		return nil, Wrap(err, KindInternal, "publishing to %s", topic)
	}
	return result.Topics, nil
}

type MessageWait struct {
	Messages []messaging.Message
	TimedOut bool
	Waited   time.Duration
}

// WaitForMessages returns and acknowledges unread messages, waiting up to timeout.
func (s *OrchestrationService) WaitForMessages(ctx context.Context, caller OrchestratorCaller, topic string, timeout time.Duration) (MessageWait, error) {
	if err := caller.check(); err != nil {
		return MessageWait{}, err
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return MessageWait{}, Errorf(KindInvalid, "topic is required")
	}
	timeout = clampWait(timeout)
	start := s.now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(orchestratorMessagePoll)
	defer poll.Stop()
	for {
		msgs, err := s.claimUnread(ctx, caller.consumer(), topic)
		if err != nil {
			return MessageWait{}, err
		}
		if len(msgs) > 0 {
			return MessageWait{Messages: msgs, Waited: s.now().Sub(start)}, nil
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			return MessageWait{TimedOut: true, Waited: s.now().Sub(start)}, nil
		case <-ctx.Done():
			return MessageWait{}, Wrap(ctx.Err(), KindUnavailable, "the wait was cancelled")
		case <-s.done:
			return MessageWait{}, Errorf(KindUnavailable, "the app is shutting down")
		}
	}
}

// claimUnread holds claimLock across the read and the acknowledgement:
// every chat in a workspace shares one consumer, so two waits that read
// before either acknowledges would both return the same message.
func (s *OrchestrationService) claimUnread(ctx context.Context, consumer, topic string) ([]messaging.Message, error) {
	select {
	case s.claimLock <- struct{}{}:
	case <-ctx.Done():
		return nil, Wrap(ctx.Err(), KindUnavailable, "the wait was cancelled")
	}
	defer func() { <-s.claimLock }()

	msgs, err := s.messages().GetUnread(ctx, consumer, topic)
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading %s", topic)
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	if err := s.messages().Acknowledge(ctx, consumer, ids); err != nil {
		return nil, Wrap(err, KindInternal, "acknowledging messages on %s", topic)
	}
	return msgs, nil
}

type SessionWait struct {
	Session  OrchestratedSession
	Matched  bool
	TimedOut bool
	Waited   time.Duration
}

// WaitForSession waits for one of states, or any change when states is empty.
func (s *OrchestrationService) WaitForSession(ctx context.Context, caller OrchestratorCaller, id string, states []string, timeout time.Duration) (SessionWait, error) {
	if err := caller.check(); err != nil {
		return SessionWait{}, err
	}
	timeout = clampWait(timeout)
	start := s.now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	var initial *string
	var last OrchestratedSession
	for {
		item, interval, err := s.sessionStatus(ctx, id)
		switch {
		case errors.Is(err, errStatusUnread):
			s.logger.Debug().Ctx(ctx).Err(err).Str("session", id).Msg("skipping a wait poll")
			interval = defaultStatusPoll
		case err != nil:
			return SessionWait{}, err
		default:
			last = item
			status := item.AgentStatus
			if !item.Running {
				status = "missing"
			}
			if initial == nil {
				initial = &status
			}
			if slices.Contains(states, status) || (len(states) == 0 && status != *initial) {
				return SessionWait{Session: item, Matched: true, Waited: s.now().Sub(start)}, nil
			}
		}
		wait := time.NewTimer(interval)
		select {
		case <-wait.C:
		case <-deadline.C:
			wait.Stop()
			return SessionWait{Session: last, TimedOut: true, Waited: s.now().Sub(start)}, nil
		case <-ctx.Done():
			wait.Stop()
			return SessionWait{}, Wrap(ctx.Err(), KindUnavailable, "the wait was cancelled")
		case <-s.done:
			wait.Stop()
			return SessionWait{}, Errorf(KindUnavailable, "the app is shutting down")
		}
	}
}

func (s *OrchestrationService) sessionStatus(ctx context.Context, id string) (OrchestratedSession, time.Duration, error) {
	sessions, interval, err := s.sessions(ctx, true, nil, true)
	if err != nil {
		return OrchestratedSession{}, 0, err
	}
	for _, sess := range sessions {
		if sess.ID == id {
			return sess, interval, nil
		}
	}
	return OrchestratedSession{}, 0, Errorf(KindNotFound, "no hive session %q", id)
}

func (s *OrchestrationService) Sleep(ctx context.Context, caller OrchestratorCaller, d time.Duration) (time.Duration, error) {
	if err := caller.check(); err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, Errorf(KindInvalid, "seconds must be positive")
	}
	d = clampWait(d)
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return d, nil
	case <-ctx.Done():
		return 0, Wrap(ctx.Err(), KindUnavailable, "the sleep was cancelled")
	case <-s.done:
		return 0, Errorf(KindUnavailable, "the app is shutting down")
	}
}

func clampWait(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return defaultOrchestratorWait
	case d > MaxOrchestratorWait:
		return MaxOrchestratorWait
	default:
		return d
	}
}
