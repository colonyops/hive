package dispatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

type cloneProtocolStub string

func (s cloneProtocolStub) CloneProtocol(context.Context) (string, error) { return string(s), nil }

func TestCloneProtocolInDraftAndAction(t *testing.T) {
	payload := []byte(`{"repo":"acme/site","url":"https://git.example.test:8443/acme/site/issues/1"}`)
	for protocol, want := range map[string]string{
		"https": "https://git.example.test:8443/acme/site.git",
		"ssh":   "ssh://git@git.example.test:8443/acme/site.git",
	} {
		t.Run(protocol, func(t *testing.T) {
			draft, err := RenderSessionDraft("Fix it", "https://wrong.test", payload, protocol)
			require.NoError(t, err)
			require.Equal(t, want, draft.Repository)
			config := &actions.LaunchSessionConfig{RepoTemplate: "{{ .ItemRemote }}", PromptTemplate: "{{ .ItemRemote }}"}
			action := actions.Action{ID: "fix", Type: "launch-session", Config: config}
			launcher := &fakeSessionLauncher{}
			executor := NewLaunchSessionExecutor(zerolog.Nop(), launcher, nil, hostEnvironment{})
			dispatcher := NewDispatcher(map[string]Executor{"launch-session": executor}, cloneProtocolStub(protocol))
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(payload, &decoded))
			_, err = dispatcher.Execute(t.Context(), action, OutputData{Key: "1", Raw: payload, Payload: decoded}, ActionInvocationInput{})
			require.NoError(t, err)
			require.Equal(t, want, launcher.calls[0].Repo)
			require.Equal(t, want, launcher.calls[0].Prompt)
			node := actions.Action{ID: "launch:triage/review", Type: "launch-session", Config: &LaunchNodeActionConfig{LaunchSessionConfig: *config}}
			_, err = dispatcher.Execute(t.Context(), node, OutputData{Key: "1", Raw: payload, Payload: decoded}, ActionInvocationInput{})
			require.NoError(t, err)
			require.Equal(t, want, launcher.calls[1].Repo)
			require.Equal(t, want, launcher.calls[1].Prompt)
			for _, explicit := range []string{"https://custom.test:9443/acme/site.git", "git@alias:acme/site.git", "/tmp/repo", "./repo"} {
				explicitDraft, err := RenderSessionDraft("Fix it", "https://github.com", []byte(`{"repo": "`+explicit+`"}`), protocol)
				require.NoError(t, err)
				require.Equal(t, explicit, explicitDraft.Repository)
				config.RepoTemplate = explicit
				got, err := RenderRepoTarget(action, "1", payload, nil, protocol)
				require.NoError(t, err)
				require.Equal(t, explicit, got)
			}
		})
	}
}

func TestSeededActionsFollowCloneProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	_, err := actions.SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	store := actions.NewActionStore(path)
	require.NotEmpty(t, store.List())
	for _, action := range store.List() {
		if action.Type != "launch-session" {
			continue
		}
		for protocol, want := range map[string]string{"https": "https://github.com/acme/site.git", "ssh": "git@github.com:acme/site.git"} {
			got, err := RenderRepoTarget(action, "1", []byte(`{"repo":"acme/site","url":"https://github.com/acme/site/issues/1"}`), nil, protocol)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	}
	// Re-seeding leaves a previously installed or customized catalog byte-identical.
	existing := []byte("version: 1\nactions:\n  - id: custom\n    repo_template: https://custom.test/repo.git\n")
	require.NoError(t, os.WriteFile(path, existing, 0o600))
	seeded, err := actions.SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	require.False(t, seeded)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, existing, got)
}

func TestActionApplicabilityUsesCloneProtocol(t *testing.T) {
	action := actions.Action{ID: "ssh-only", Type: "launch-session", Config: &actions.LaunchSessionConfig{
		RepoTemplate: `{{ if eq .ItemRemote "git@github.com:acme/site.git" }}{{ .ItemRemote }}{{ end }}`,
	}}
	item := DecodedActionItem{ID: "1", Payload: []byte(`{"repo":"acme/site","url":"https://github.com/acme/site/issues/1"}`)}
	ok, _ := ActionApplicability(action, item, "https")
	require.False(t, ok)
	ok, reason := ActionApplicability(action, item, "ssh")
	require.True(t, ok, reason)
}

func TestClipboardCloneProtocolAtEachDispatch(t *testing.T) {
	protocol := cloneProtocolStub("https")
	dispatcher := NewDispatcher(map[string]Executor{"clipboard": NewClipboardExecutor()}, &protocol)
	action := actions.Action{ID: "copy", Type: "clipboard", Config: &actions.ClipboardConfig{TextTemplate: "{{ .ItemRemote }}"}}
	payload := []byte(`{"repo":"acme/site","url":"https://github.com/acme/site/issues/1"}`)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	for _, preference := range []string{"https", "ssh"} {
		protocol = cloneProtocolStub(preference)
		want := "https://github.com/acme/site.git"
		if preference == "ssh" {
			want = "git@github.com:acme/site.git"
		}
		result, err := dispatcher.Execute(t.Context(), action, OutputData{Raw: payload, Payload: decoded}, ActionInvocationInput{})
		require.NoError(t, err)
		require.Equal(t, want, result.Outcome.Clipboard.Text)
		direct, err := RenderClipboardText(action, "1", payload, nil, preference)
		require.NoError(t, err)
		require.Equal(t, want, direct)
	}
}
