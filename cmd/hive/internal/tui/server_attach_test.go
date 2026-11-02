package tui

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	act "github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/serverattach"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

type fakeServerAPI struct {
	sessionapi.API
	risk    sessionsvc.Risk
	riskErr error
	create  sessionsvc.CreateOptions
}

func (a *fakeServerAPI) GetSession(_ context.Context, id string) (session.Session, error) {
	return session.Session{ID: id, Name: "demo", Slug: "demo", State: session.StateActive}, nil
}

func (a *fakeServerAPI) CreateSession(_ context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	a.create = opts
	return &session.Session{ID: "new", Name: opts.Name}, nil
}

func (a *fakeServerAPI) CheckSessionRisk(context.Context, string) (sessionsvc.Risk, error) {
	return a.risk, a.riskErr
}

type fakeAttacher struct {
	api      *fakeServerAPI
	startErr error
}

func (f fakeAttacher) StartServerSession(context.Context, string, session.Session) (server.Host, error) {
	return server.Host{Name: "box"}, f.startErr
}

func (f fakeAttacher) ServerAPI(string) (sessionapi.API, error) { return f.api, nil }

func newServerAttachModel(attacher fakeAttacher) Model {
	m := newActionTestModel()
	m.logger = zerolog.Nop()
	m.sessions = attacher.api
	m.serverAttach = attacher
	return m
}

func TestStartServerAttach_ReportsACompletedSessionAsInformation(t *testing.T) {
	api := &fakeServerAPI{}
	completed := fmt.Errorf("session demo on server box: %w", serverattach.ErrSessionCompleted)
	m := newServerAttachModel(fakeAttacher{api: api, startErr: completed})

	msg := m.startServerAttach(session.Ref{Server: "box", ID: "abc"}, Action{})()

	done, ok := msg.(serverActionDoneMsg)
	require.True(t, ok)
	require.NoError(t, done.err)
	assert.Contains(t, done.message, "no terminal to attach to")
}

func TestStartServerAttach_TargetsTheSelectedWindow(t *testing.T) {
	m := newServerAttachModel(fakeAttacher{api: &fakeServerAPI{}})

	msg := m.startServerAttach(session.Ref{Server: "box", ID: "abc"}, Action{TmuxWindow: "tests", Exit: true})()

	ready, ok := msg.(serverAttachReadyMsg)
	require.True(t, ok)
	assert.Equal(t, "=demo:tests", ready.target)
	assert.True(t, ready.exit)
}

func TestServerActionDone_KeepsAModalTheUserOpened(t *testing.T) {
	m := newServerAttachModel(fakeAttacher{api: &fakeServerAPI{}})
	m.state = stateConfirming
	pending := Action{SessionID: "other", Type: act.TypeDelete}
	m.modals.Pending = pending

	result, _ := m.handleServerActionDone(serverActionDoneMsg{actionCompleteMsg{message: "Session demo created on box"}})

	got := result.(Model)
	assert.Equal(t, stateConfirming, got.state)
	assert.Equal(t, pending.SessionID, got.modals.Pending.SessionID)
	assert.Equal(t, []string{"Session demo created on box"}, notificationMessages(got))
}

func TestServerLaunchOptions_DoNotOpenTheFormOverAModal(t *testing.T) {
	m := newServerAttachModel(fakeAttacher{api: &fakeServerAPI{}})
	m.state = stateConfirming

	result, _ := m.openServerNewSessionForm(serverLaunchOptionsMsg{
		server:  "box",
		options: sessionsvc.LaunchOptions{Repositories: []sessionsvc.LaunchRepository{{Name: "site", Remote: "r"}}},
	})

	got := result.(Model)
	assert.Equal(t, stateConfirming, got.state)
	assert.Nil(t, got.modals.NewSession)
}

func TestCreateOnServer_SendsTheServersCheckoutForRuleFiles(t *testing.T) {
	api := &fakeServerAPI{}
	m := newServerAttachModel(fakeAttacher{api: api})
	repos := serverRepos(sessionsvc.LaunchOptions{
		Repositories: []sessionsvc.LaunchRepository{{Name: "site", Remote: "r", Source: "/srv/site"}},
	})

	msg := m.createOnServer("demo", repos[0], "", "box")()

	_, ok := msg.(serverActionDoneMsg)
	require.True(t, ok)
	assert.Equal(t, "/srv/site", api.create.Source)
}

func TestSessionRiskChecked_AsksForTheTypedConfirmWhenTheCheckFails(t *testing.T) {
	m := newServerAttachModel(fakeAttacher{api: &fakeServerAPI{}})

	result, _ := m.handleSessionRiskChecked(sessionRiskCheckedMsg{
		action: Action{Type: act.TypeDelete, SessionID: "box:abc"},
		err:    errors.New("box did not answer in 5s"),
	})

	got := result.(Model)
	assert.Equal(t, stateConfirming, got.state)
	assert.True(t, got.modals.Confirm.dangerous)
	assert.Contains(t, got.modals.Confirm.message, "box did not answer in 5s")
}

func TestRenderWithFormData_RefusesALocalCommandForAServerSession(t *testing.T) {
	handler := NewKeybindingResolver(zerolog.Nop(), sessionsKBs(nil), commandSetFromMap(nil), testRenderer)
	cmd := config.UserCommand{Sh: "code {{ .Path }}", Help: "open"}

	a := handler.RenderWithFormData("open", cmd, session.Session{ID: "box:abc", Name: "demo", Path: "/srv/x"}, nil, nil, nil)

	require.ErrorContains(t, a.Err, "on server box")
	assert.Empty(t, a.ShellCmd)
}
