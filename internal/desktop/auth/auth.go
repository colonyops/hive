// Package auth implements GitHub authentication for the Hive desktop app:
// the Wails auth service, its live (device flow + PAT) and mock backends,
// and the wire types the onboarding UI consumes. Desktop-only code lives
// under internal/desktop; the GitHub client itself is internal/github.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/github"
)

// Auth states on the wire. Strings, not enums: the frontend narrows them.
const (
	StateUnauthenticated = "unauthenticated"
	StateAuthenticated   = "authenticated"
)

// EnvGitHubClientID overrides the OAuth app client ID used by the device
// flow, e.g. to test against a different OAuth app registration.
const EnvGitHubClientID = "HIVE_GITHUB_CLIENT_ID"

// defaultClientID is the registered Hive Desktop OAuth app. Client IDs are
// public; the device flow uses no client secret.
const defaultClientID = "Ov23likA3JPBPkYbMGu4"

// deviceFlowScopes: repo covers PR/issue search on private repos;
// notifications covers the inbox feed.
var deviceFlowScopes = []string{"repo", "notifications"}

// Status is the authentication state shown to the frontend.
type Status struct {
	State     string `json:"state"` // StateUnauthenticated | StateAuthenticated
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	// Message carries a user-facing problem description (invalid stored
	// token, device flow failure) without changing the state machine.
	Message string `json:"message"`
}

// DeviceFlowInfo is the pending device authorization the onboarding screen
// renders: the user opens VerificationURI and enters UserCode.
type DeviceFlowInfo struct {
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
}

// Backend implements authentication for Service. liveAuth talks to
// GitHub; mockAuth drives deterministic onboarding for tests and e2e.
type Backend interface {
	Status(ctx context.Context) Status
	StartDeviceFlow(ctx context.Context) (DeviceFlowInfo, error)
	CancelDeviceFlow()
	SetToken(ctx context.Context, token string) (Status, error)
	SignOut() error
}

// Service is the Wails service exposing authentication to the frontend.
// State changes are pushed via the auth:updated event; the frontend re-reads
// Status on receipt.
type Service struct {
	backend Backend
}

func NewService(backend Backend) *Service {
	return &Service{backend: backend}
}

func (s *Service) Status() Status {
	return s.backend.Status(context.Background())
}

func (s *Service) StartDeviceFlow() (DeviceFlowInfo, error) {
	return s.backend.StartDeviceFlow(context.Background())
}

func (s *Service) CancelDeviceFlow() {
	s.backend.CancelDeviceFlow()
}

func (s *Service) SetToken(token string) (Status, error) {
	return s.backend.SetToken(context.Background(), token)
}

func (s *Service) SignOut() error {
	return s.backend.SignOut()
}

// ── Live backend ─────────────────────────────────────────────────────────────

type liveAuth struct {
	client   *github.Client
	tokens   github.TokenStore
	clientID string
	// onChange is called after every state transition (device flow grant or
	// failure, token set, sign-out). main.go wires it to the auth:updated
	// event emit.
	onChange func()

	mu         sync.Mutex
	flowCancel context.CancelFunc
	cached     *Status
}

func NewLiveBackend(client *github.Client, tokens github.TokenStore, onChange func()) Backend {
	clientID := os.Getenv(EnvGitHubClientID)
	if clientID == "" {
		clientID = defaultClientID
	}
	return &liveAuth{client: client, tokens: tokens, clientID: clientID, onChange: onChange}
}

func (a *liveAuth) Status(ctx context.Context) Status {
	a.mu.Lock()
	if a.cached != nil {
		defer a.mu.Unlock()
		return *a.cached
	}
	a.mu.Unlock()

	token, err := a.tokens.Token()
	if err != nil {
		return Status{State: StateUnauthenticated, Message: err.Error()}
	}
	if token == "" {
		return Status{State: StateUnauthenticated}
	}

	user, err := a.client.WithTokenCopy(token).User(ctx)
	switch {
	case errors.Is(err, github.ErrUnauthorized):
		return Status{State: StateUnauthenticated, Message: "Stored GitHub token is no longer valid."}
	case err != nil:
		// Unreachable/rate limited with a stored token: optimistically
		// authenticated so the feed shell (with its unreachable state) shows
		// instead of onboarding.
		return Status{State: StateAuthenticated, Message: err.Error()}
	}

	status := authenticatedStatus(user)
	a.setCached(status)
	return status
}

func (a *liveAuth) StartDeviceFlow(ctx context.Context) (DeviceFlowInfo, error) {
	auth, err := a.client.StartDeviceFlow(ctx, a.clientID, deviceFlowScopes)
	if err != nil {
		return DeviceFlowInfo{}, fmt.Errorf("start device flow: %w", err)
	}

	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(auth.ExpiresIn)*time.Second)
	a.mu.Lock()
	if a.flowCancel != nil {
		a.flowCancel()
	}
	a.flowCancel = cancel
	// A fresh attempt supersedes any earlier failure message.
	a.cached = nil
	a.mu.Unlock()

	go a.pollFlow(pollCtx, auth)

	return DeviceFlowInfo{UserCode: auth.UserCode, VerificationURI: auth.VerificationURI}, nil
}

func (a *liveAuth) pollFlow(ctx context.Context, auth github.DeviceAuth) {
	token, err := a.client.PollDeviceFlow(ctx, a.clientID, auth)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		a.setCached(Status{State: StateUnauthenticated, Message: flowFailureMessage(err)})
		a.notify()
		return
	}

	// Not the poll context: its device-code deadline may be about to fire,
	// and validating a just-granted token must not race it.
	a.adoptToken(context.Background(), token)
}

// flowFailureMessage maps device-flow failures onto user-facing text. The
// local poll deadline (ExpiresIn) usually fires before GitHub ever reports
// expired_token, so DeadlineExceeded means the code expired unused.
func flowFailureMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "The sign-in code expired before authorization. Start again to get a fresh code."
	}
	return err.Error()
}

func (a *liveAuth) SetToken(ctx context.Context, token string) (Status, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Status{}, fmt.Errorf("token is empty")
	}

	user, err := a.client.WithTokenCopy(token).User(ctx)
	if errors.Is(err, github.ErrUnauthorized) {
		return Status{}, fmt.Errorf("GitHub rejected the token")
	}
	if err != nil {
		return Status{}, fmt.Errorf("validate token: %w", err)
	}

	if err := a.tokens.SetToken(token); err != nil {
		return Status{}, err
	}
	status := authenticatedStatus(user)
	a.setCached(status)
	a.notify()
	return status, nil
}

// adoptToken validates and stores a token granted by the device flow, then
// notifies. Errors surface through the cached status: the poll goroutine has
// no caller to return them to.
func (a *liveAuth) adoptToken(ctx context.Context, token string) {
	user, err := a.client.WithTokenCopy(token).User(ctx)
	if err != nil {
		a.setCached(Status{State: StateUnauthenticated, Message: fmt.Sprintf("validate granted token: %v", err)})
		a.notify()
		return
	}
	if err := a.tokens.SetToken(token); err != nil {
		a.setCached(Status{State: StateUnauthenticated, Message: err.Error()})
		a.notify()
		return
	}
	a.setCached(authenticatedStatus(user))
	a.notify()
}

func (a *liveAuth) CancelDeviceFlow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.flowCancel != nil {
		a.flowCancel()
		a.flowCancel = nil
	}
}

func (a *liveAuth) SignOut() error {
	a.CancelDeviceFlow()
	if err := a.tokens.DeleteToken(); err != nil {
		return err
	}
	status := Status{State: StateUnauthenticated}
	// The env override outranks the keychain, so deleting the keychain entry
	// cannot revoke access; say so instead of silently bouncing back to
	// authenticated on the next Status read.
	if os.Getenv(github.EnvToken) != "" {
		status.Message = "Signed out, but the " + github.EnvToken + " environment override is still set and keeps this session authenticated."
	}
	a.setCached(status)
	a.notify()
	return nil
}

// setCached pins the status Status() returns. Failure statuses are cached
// too: auth:updated carries no payload, so the Message must survive until
// the frontend's follow-up Status() read.
func (a *liveAuth) setCached(status Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cached = &status
}

func (a *liveAuth) notify() {
	if a.onChange != nil {
		a.onChange()
	}
}

func authenticatedStatus(user github.User) Status {
	return Status{
		State:     StateAuthenticated,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}
}

// ── Mock backend ─────────────────────────────────────────────────────────────

// mockAuth drives the onboarding flow deterministically and offline. In
// "feed" mock mode it starts authenticated; in "onboarding" mode it starts
// signed out and grants the fake device flow after a short delay.
type mockAuth struct {
	onChange func()

	mu     sync.Mutex
	status Status
	timer  *time.Timer
	// flowSeq invalidates a pending grant whose timer already fired but is
	// blocked on mu when CancelDeviceFlow runs: timer.Stop() returns false
	// then, so the callback must re-check it is still the current flow.
	flowSeq int
}

const mockGrantDelay = 1500 * time.Millisecond

func NewMockBackend(authenticated bool, onChange func()) Backend {
	status := Status{State: StateUnauthenticated}
	if authenticated {
		status = mockAuthenticatedStatus()
	}
	return &mockAuth{status: status, onChange: onChange}
}

func mockAuthenticatedStatus() Status {
	return Status{State: StateAuthenticated, Login: "hayden", Name: "Hayden"}
}

func (a *mockAuth) Status(context.Context) Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *mockAuth) StartDeviceFlow(context.Context) (DeviceFlowInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timer != nil {
		a.timer.Stop()
	}
	a.flowSeq++
	seq := a.flowSeq
	a.timer = time.AfterFunc(mockGrantDelay, func() {
		a.mu.Lock()
		if a.flowSeq != seq {
			a.mu.Unlock()
			return
		}
		a.status = mockAuthenticatedStatus()
		a.mu.Unlock()
		if a.onChange != nil {
			a.onChange()
		}
	})
	return DeviceFlowInfo{UserCode: "7B4C-Q22F", VerificationURI: "https://github.com/login/device"}, nil
}

func (a *mockAuth) CancelDeviceFlow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.flowSeq++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

func (a *mockAuth) SetToken(_ context.Context, token string) (Status, error) {
	if strings.TrimSpace(token) == "" {
		return Status{}, fmt.Errorf("token is empty")
	}
	a.mu.Lock()
	a.flowSeq++ // a pending device grant must not re-fire over an explicit token
	a.status = mockAuthenticatedStatus()
	a.mu.Unlock()
	if a.onChange != nil {
		a.onChange()
	}
	return mockAuthenticatedStatus(), nil
}

func (a *mockAuth) SignOut() error {
	a.CancelDeviceFlow()
	a.mu.Lock()
	a.status = Status{State: StateUnauthenticated}
	a.mu.Unlock()
	if a.onChange != nil {
		a.onChange()
	}
	return nil
}
