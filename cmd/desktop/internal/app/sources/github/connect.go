package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// Connection states on the wire. Connected/disconnected, not
// authenticated/unauthenticated: GitHub is one connector among several, and
// nothing about the app is gated on holding a credential for it. The frontend
// narrows these strings, so they are strings rather than an enum.
const (
	StateDisconnected = "disconnected"
	StateConnected    = "connected"
)

// EnvClientID overrides the OAuth app client ID used by the device flow, e.g.
// to test against a different OAuth app registration.
const EnvClientID = "HIVE_GITHUB_CLIENT_ID"

// defaultClientID is the registered Hive Desktop OAuth app. Client IDs are
// public; the device flow uses no client secret.
const defaultClientID = "Ov23likA3JPBPkYbMGu4"

// deviceFlowScopes: repo covers PR/issue search on private repos;
// notifications covers the inbox feed.
var deviceFlowScopes = []string{"repo", "notifications"}

// ConnectionStatus is this connector's own connection state. It is the
// connector's, not the app's: Login/Name/AvatarURL describe the connected
// GitHub account and belong on an Integrations card, not in app chrome.
type ConnectionStatus struct {
	State     string `json:"state"` // StateDisconnected | StateConnected
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	// Message carries a user-facing problem description (invalid stored
	// token, device flow failure) without changing the state machine.
	Message string `json:"message"`
}

// DeviceFlowInfo is the pending device authorization the connect screen
// renders: the user opens VerificationURI and enters UserCode.
type DeviceFlowInfo struct {
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
}

// Connection acquires and releases this connector's credentials.
// liveConnection talks to GitHub; mockConnection drives deterministic
// connect/disconnect for tests and e2e.
//
// Acquisition is the only provider-specific half of credentials — lookup is
// generic and lives in app/credentials — which is why this interface is
// declared by the connector that implements it rather than by the app.
// A connector with no state machine (an API token pasted into a
// secret-marked config field) needs none of it.
type Connection interface {
	Status(ctx context.Context) ConnectionStatus
	StartDeviceFlow(ctx context.Context) (DeviceFlowInfo, error)
	CancelDeviceFlow()
	SetToken(ctx context.Context, token string) (ConnectionStatus, error)
	Disconnect() error
}

// ── Live connection ──────────────────────────────────────────────────────────

type liveConnection struct {
	client *ghclient.Client
	// creds is the credential store. A token is keyed by the login it belongs
	// to, so storing one needs the user lookup that validation already
	// performs.
	creds    credentials.Store
	clientID string
	// onChange is called after every state transition (device flow grant or
	// failure, token set, disconnect). app.New wires it to the fetch-cache
	// drop and the ConnectionUpdated publish.
	onChange func()

	mu         sync.Mutex
	flowCancel context.CancelFunc
	cached     *ConnectionStatus
}

// NewLiveConnection builds the real GitHub connection over a credential
// store.
func NewLiveConnection(client *ghclient.Client, creds credentials.Store, onChange func()) Connection {
	clientID := os.Getenv(EnvClientID)
	if clientID == "" {
		clientID = defaultClientID
	}
	return &liveConnection{
		client:   client,
		creds:    creds,
		clientID: clientID,
		onChange: onChange,
	}
}

// connectedRefs is every stored GitHub credential. Several are representable
// now that credentials are keyed by account; this reports one connected user,
// which is the shape the Integrations screen widens.
func (a *liveConnection) connectedRefs() ([]credentials.Ref, error) {
	return credentials.ListProvider(a.creds, Provider)
}

// token is the token to validate against, preferring the environment
// override so a headless run needs no stored credential at all.
func (a *liveConnection) token() (string, error) {
	if value := os.Getenv(credentials.EnvOverrideName(Provider)); value != "" {
		return value, nil
	}
	refs, err := a.connectedRefs()
	if err != nil || len(refs) == 0 {
		return "", err
	}
	return credentials.Resolve(a.creds, refs[0])
}

func (a *liveConnection) Status(ctx context.Context) ConnectionStatus {
	a.mu.Lock()
	if a.cached != nil {
		defer a.mu.Unlock()
		return *a.cached
	}
	a.mu.Unlock()

	token, err := a.token()
	if err != nil {
		return ConnectionStatus{State: StateDisconnected, Message: err.Error()}
	}
	if token == "" {
		return ConnectionStatus{State: StateDisconnected}
	}

	user, err := a.client.WithTokenCopy(token).User(ctx)
	switch {
	case errors.Is(err, sourcehttp.ErrUnauthorized):
		return ConnectionStatus{State: StateDisconnected, Message: "Stored GitHub token is no longer valid."}
	case err != nil:
		// Unreachable/rate limited with a stored token: optimistically
		// connected so the feed shell (with its unreachable state) shows
		// instead of an offer to connect.
		return ConnectionStatus{State: StateConnected, Message: err.Error()}
	}

	status := connectedStatus(user)
	a.setCached(status)
	return status
}

func (a *liveConnection) StartDeviceFlow(ctx context.Context) (DeviceFlowInfo, error) {
	auth, err := a.client.StartDeviceFlow(ctx, a.clientID, deviceFlowScopes)
	if err != nil {
		return DeviceFlowInfo{}, fmt.Errorf("start device flow: %w", err)
	}

	// Detached from the caller's context, not rooted at Background: the poll
	// must outlive the StartDeviceFlow RPC that began it, but it should still
	// carry that call's values.
	pollCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(auth.ExpiresIn)*time.Second)
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

func (a *liveConnection) pollFlow(ctx context.Context, auth ghclient.DeviceAuth) {
	token, err := a.client.PollDeviceFlow(ctx, a.clientID, auth)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		a.setCached(ConnectionStatus{State: StateDisconnected, Message: flowFailureMessage(err)})
		a.notify()
		return
	}

	// Not the poll context: its device-code deadline may be about to fire,
	// and validating a just-granted token must not race it.
	a.adoptToken(context.WithoutCancel(ctx), token)
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

func (a *liveConnection) SetToken(ctx context.Context, token string) (ConnectionStatus, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ConnectionStatus{}, fmt.Errorf("token is empty")
	}

	user, err := a.client.WithTokenCopy(token).User(ctx)
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return ConnectionStatus{}, fmt.Errorf("GitHub rejected the token")
	}
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("validate token: %w", err)
	}

	if err := a.creds.Set(credentials.Ref{Provider: Provider, Account: user.Login}, token); err != nil {
		return ConnectionStatus{}, err
	}
	status := connectedStatus(user)
	a.setCached(status)
	a.notify()
	return status, nil
}

// adoptToken validates and stores a token granted by the device flow, then
// notifies. Errors surface through the cached status: the poll goroutine has
// no caller to return them to.
func (a *liveConnection) adoptToken(ctx context.Context, token string) {
	user, err := a.client.WithTokenCopy(token).User(ctx)
	if err != nil {
		a.setCached(ConnectionStatus{State: StateDisconnected, Message: fmt.Sprintf("validate granted token: %v", err)})
		a.notify()
		return
	}
	if err := a.creds.Set(credentials.Ref{Provider: Provider, Account: user.Login}, token); err != nil {
		a.setCached(ConnectionStatus{State: StateDisconnected, Message: err.Error()})
		a.notify()
		return
	}
	a.setCached(connectedStatus(user))
	a.notify()
}

func (a *liveConnection) CancelDeviceFlow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.flowCancel != nil {
		a.flowCancel()
		a.flowCancel = nil
	}
}

func (a *liveConnection) Disconnect() error {
	a.CancelDeviceFlow()
	// Every connected account, not just one: disconnecting means "this app
	// holds no GitHub credentials", and leaving one behind would keep the feed
	// fetching as an account the user believes they removed.
	refs, err := a.connectedRefs()
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := a.creds.Delete(ref); err != nil {
			return err
		}
	}
	status := ConnectionStatus{State: StateDisconnected}
	// The env override outranks the keychain, so deleting the keychain entry
	// cannot revoke access; say so instead of silently bouncing back to
	// connected on the next Status read.
	if envName := credentials.EnvOverrideName(Provider); os.Getenv(envName) != "" {
		status.Message = "Disconnected, but the " + envName + " environment override is still set and keeps this session connected."
	}
	a.setCached(status)
	a.notify()
	return nil
}

// setCached pins the status Status() returns. Failure statuses are cached
// too: connection:updated carries only the provider, so the Message must
// survive until the frontend's follow-up Status() read.
func (a *liveConnection) setCached(status ConnectionStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cached = &status
}

func (a *liveConnection) notify() {
	if a.onChange != nil {
		a.onChange()
	}
}

func connectedStatus(user ghclient.User) ConnectionStatus {
	return ConnectionStatus{
		State:     StateConnected,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}
}

// ── Mock connection ──────────────────────────────────────────────────────────

// mockConnection drives the connect flow deterministically and offline. In
// "feed" mock mode it starts connected; in "onboarding" mode it starts
// disconnected and grants the fake device flow after a short delay.
type mockConnection struct {
	// creds is written on grant and cleared on disconnect, exactly as the live
	// connection does. A mock that only flipped a status flag would leave the
	// credential store empty, and everything downstream that resolves an
	// account — a source node's credential, seeding a workspace — would fail
	// in mock mode only.
	creds    credentials.Store
	onChange func()

	mu     sync.Mutex
	status ConnectionStatus
	timer  *time.Timer
	// flowSeq invalidates a pending grant whose timer already fired but is
	// blocked on mu when CancelDeviceFlow runs: timer.Stop() returns false
	// then, so the callback must re-check it is still the current flow.
	flowSeq int
}

const mockGrantDelay = 1500 * time.Millisecond

// NewMockConnection builds the offline connection the mock modes use.
func NewMockConnection(connected bool, creds credentials.Store, onChange func()) Connection {
	a := &mockConnection{creds: creds, status: ConnectionStatus{State: StateDisconnected}, onChange: onChange}
	if connected {
		a.status = mockConnectedStatus()
		a.connect()
	}
	return a
}

// mockLogin is the account every mock mode is connected as.
const mockLogin = "octocat"

func mockConnectedStatus() ConnectionStatus {
	return ConnectionStatus{State: StateConnected, Login: mockLogin, Name: "Octocat"}
}

// connect stores the fake credential. Errors are ignored: the store is always
// the in-memory one in mock modes, where a write cannot fail.
func (a *mockConnection) connect() {
	if a.creds != nil {
		_ = a.creds.Set(credentials.Ref{Provider: Provider, Account: mockLogin}, "mock-token")
	}
}

func (a *mockConnection) disconnect() {
	if a.creds != nil {
		_ = a.creds.Delete(credentials.Ref{Provider: Provider, Account: mockLogin})
	}
}

func (a *mockConnection) Status(context.Context) ConnectionStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *mockConnection) StartDeviceFlow(context.Context) (DeviceFlowInfo, error) {
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
		a.status = mockConnectedStatus()
		a.connect()
		a.mu.Unlock()
		if a.onChange != nil {
			a.onChange()
		}
	})
	return DeviceFlowInfo{UserCode: "7B4C-Q22F", VerificationURI: "https://github.com/login/device"}, nil
}

func (a *mockConnection) CancelDeviceFlow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.flowSeq++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

func (a *mockConnection) SetToken(_ context.Context, token string) (ConnectionStatus, error) {
	if strings.TrimSpace(token) == "" {
		return ConnectionStatus{}, fmt.Errorf("token is empty")
	}
	a.mu.Lock()
	a.flowSeq++ // a pending device grant must not re-fire over an explicit token
	a.status = mockConnectedStatus()
	a.connect()
	a.mu.Unlock()
	if a.onChange != nil {
		a.onChange()
	}
	return mockConnectedStatus(), nil
}

func (a *mockConnection) Disconnect() error {
	a.CancelDeviceFlow()
	a.mu.Lock()
	a.status = ConnectionStatus{State: StateDisconnected}
	a.disconnect()
	a.mu.Unlock()
	if a.onChange != nil {
		a.onChange()
	}
	return nil
}
