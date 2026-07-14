package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/workspace"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/sourcepicker"
	sessionsview "github.com/colonyops/hive/internal/tui/views/sessions"
)

// stubSource is a minimal sources.Source for registry-backed wiring tests.
type stubSource struct {
	id          string
	detail      sources.Detail
	detailErr   error
	fetchParams *[]sources.FetchDetailParams
}

func (s stubSource) Name() string                   { return s.id }
func (s stubSource) Available(context.Context) bool { return true }
func (s stubSource) Initialize(context.Context) (sources.Manifest, error) {
	return sources.Manifest{ID: s.id}, nil
}

func (s stubSource) Search(context.Context, sources.SearchParams) (sources.SearchResult, error) {
	return sources.SearchResult{}, nil
}

func (s stubSource) FetchDetail(_ context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	if s.fetchParams != nil {
		*s.fetchParams = append(*s.fetchParams, params)
	}
	if s.detailErr != nil {
		return sources.Detail{}, s.detailErr
	}
	return s.detail, nil
}

func registryWith(t *testing.T, ids ...string) *sources.Registry {
	t.Helper()
	reg := sources.NewRegistry()
	for _, id := range ids {
		require.NoError(t, reg.Register(id, sources.BackendGithub, stubSource{id: id}, sources.TemplateConfig{}, id))
	}
	return reg
}

func TestResolveSourceID(t *testing.T) {
	tests := []struct {
		name    string
		reg     *sources.Registry
		args    []string
		want    string
		wantErr string
	}{
		{"explicit arg wins", registryWith(t, "issues", "prs"), []string{"prs"}, "prs", ""},
		{"explicit view resolves", registryWith(t, "issues", "prs", "triage"), []string{"triage"}, "triage", ""},
		{"unknown explicit id errors", registryWith(t, "issues", "prs", "triage"), []string{"missing"}, "", "source \"missing\" is not configured"},
		{"explicit arg without registry", nil, []string{"issues"}, "issues", ""},
		{"sole source defaults", registryWith(t, "issues"), nil, "issues", ""},
		{"nil registry", nil, nil, "", "no sources are configured"},
		{"empty registry", registryWith(t), nil, "", "no sources are configured"},
		{"multiple sources are ambiguous", registryWith(t, "issues", "prs"), nil, "", "multiple sources configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{sourceRegistry: tt.reg}
			got, err := m.resolveSourceID(tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenSourcePicker_ConfiguredViewTab(t *testing.T) {
	m := Model{
		cfg: &config.Config{Sources: config.SourcesConfig{Views: []config.SourceViewConfig{{
			Name: "triage", Base: "issues", Query: "label:triage",
		}}}},
		sourceRegistry: registryWith(t, "issues", "prs", "triage"),
		modals:         NewModalCoordinator(),
		width:          100,
		height:         24,
	}

	model, cmd := m.openSourcePicker("triage", sourcePickerScope{Remote: "https://github.com/colonyops/hive"})
	require.NotNil(t, cmd)
	opened := model.(Model)
	require.Equal(t, stateSourcePicker, opened.state)
	require.NotNil(t, opened.modals.SourcePicker)

	view := opened.modals.SourcePicker.View()
	assert.Contains(t, view, "issues")
	assert.Contains(t, view, "prs")
	assert.Contains(t, view, "triage")

	picker, searchCmd := opened.modals.SourcePicker.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	assert.Nil(t, searchCmd, "configured view tabs disable search")
	assert.Contains(t, picker.View(), "search disabled for saved views")
}

func TestHandleSourceSelection_InstantVsGlobalForm(t *testing.T) {
	global := sourcepicker.Result{
		Item: sources.Item{
			ID: "42", URI: "https://git.example.com/acme/widgets/issues/42",
			Fields: map[string]any{"repo": "acme/widgets", "number": 42},
		},
		SourceID:  "cross-repo",
		Templates: sources.TemplateConfig{Name: "issue-{{ .Fields.number }}"},
	}
	builtin := global
	builtin.SourceID = "issues"
	scoped := global
	scoped.SourceID = "triage"

	newModel := func() Model {
		return Model{
			cfg: &config.Config{Sources: config.SourcesConfig{Views: []config.SourceViewConfig{
				{Name: "cross-repo", Base: "issues", Query: "org:acme"},
				{Name: "triage", Base: "issues", Query: "label:triage", Scope: "acme/widgets"},
			}}},
			modals:             NewModalCoordinator(),
			pendingSourceScope: sourcePickerScope{Search: "fallback/repo", Remote: "git@github.com:fallback/repo.git"},
		}
	}

	t.Run("built-in stays on instant background path", func(t *testing.T) {
		m := newModel()
		picker := sourcepicker.New(nil, "", "", "", 80, 24)
		m.modals.SourcePicker = &picker
		model, cmd := m.handleSourceSelection([]sourcepicker.Result{builtin})
		updated := model.(Model)
		require.NotNil(t, cmd)
		assert.Nil(t, updated.modals.SourcePicker)
		assert.Nil(t, updated.pendingSourceForm)
	})

	t.Run("repo-scoped view stays on instant background path", func(t *testing.T) {
		m := newModel()
		model, cmd := m.handleSourceSelection([]sourcepicker.Result{scoped})
		updated := model.(Model)
		require.NotNil(t, cmd)
		assert.Nil(t, updated.pendingSourceForm)
	})

	t.Run("global view opens form with synthetic repo and rendered name", func(t *testing.T) {
		m := newModel()
		picker := sourcepicker.New(nil, "", "", "", 80, 24)
		m.modals.SourcePicker = &picker
		model, cmd := m.handleSourceSelection([]sourcepicker.Result{global})
		updated := model.(Model)
		assert.Nil(t, cmd)
		assert.Equal(t, stateCreatingSession, updated.state)
		require.NotNil(t, updated.pendingSourceForm)
		require.NotNil(t, updated.modals.NewSession)
		formResult := updated.modals.NewSession.Result()
		assert.Equal(t, "https://git.example.com/acme/widgets", formResult.Repo.Remote)
		assert.Equal(t, "acme/widgets", formResult.Repo.Name)
		assert.Equal(t, "issue-42", updated.modals.NewSession.nameInput.Value())
	})

	t.Run("multiple global marks are retained for one-at-a-time selection", func(t *testing.T) {
		m := newModel()
		picker := sourcepicker.New(nil, "", "", "", 80, 24)
		m.modals.SourcePicker = &picker
		second := global
		second.Item.ID = "43"
		model, cmd := m.handleSourceSelection([]sourcepicker.Result{global, second})
		updated := model.(Model)
		assert.Nil(t, cmd)
		assert.Equal(t, stateSourcePicker, updated.state)
		assert.NotNil(t, updated.modals.SourcePicker, "picker and its marks remain available")
		assert.Nil(t, updated.pendingSourceForm)
	})
}

func TestOpenSourceSessionForm_UsesExistingOrSyntheticRepo(t *testing.T) {
	view := &sessionsview.View{}
	repos := []workspace.DiscoveredRepo{
		{Path: "/work/other", Name: "other", Remote: "git@github.com:acme/other.git"},
		{Path: "/work/widgets", Name: "widgets", Remote: "git@git.example.com:acme/widgets.git"},
	}
	setUnexportedField(t, view, "discoveredRepos", repos)

	result := sourcepicker.Result{
		Item:      sources.Item{ID: "7", URI: "https://git.example.com/acme/widgets/pulls/7", Fields: map[string]any{"repo": "acme/widgets"}},
		SourceID:  "global",
		Templates: sources.TemplateConfig{Name: "pr-{{ .ID }}"},
	}
	m := Model{cfg: &config.Config{}, modals: NewModalCoordinator(), sessionsView: view}

	model, _ := m.openSourceSessionForm(result)
	form := model.(Model).modals.NewSession
	require.NotNil(t, form)
	assert.Len(t, form.repos, 2, "an existing exact remote is not duplicated")
	assert.Equal(t, 1, form.repoSelect.SelectedIndex())
	assert.Equal(t, "git@git.example.com:acme/widgets.git", form.Result().Repo.Remote)

	result.Item.Fields["repo"] = "acme/new-repo"
	result.Item.URI = "https://git.example.com/acme/new-repo/issues/7"
	model, _ = m.openSourceSessionForm(result)
	form = model.(Model).modals.NewSession
	require.NotNil(t, form)
	assert.Len(t, form.repos, 3)
	assert.Equal(t, 0, form.repoSelect.SelectedIndex(), "synthetic item repo is prepended and selected")
	assert.Equal(t, "acme/new-repo", form.Result().Repo.Name)
}

func TestResolveItemRemote(t *testing.T) {
	view := &sessionsview.View{}
	setUnexportedField(t, view, "discoveredRepos", []workspace.DiscoveredRepo{
		{Path: "/work/widgets", Remote: "git@git.example.com:acme/widgets.git"},
	})
	setUnexportedField(t, view, "allSessions", []session.Session{
		{Name: "session-repo", Path: "/sessions/repo", Remote: "ssh://git@code.example.net/acme/session-repo.git"},
	})
	m := Model{sessionsView: view}

	tests := []struct {
		name string
		item sources.Item
		want string
		ok   bool
	}{
		{
			name: "reuses exact discovered SSH remote",
			item: sources.Item{URI: "https://git.example.com/acme/widgets/issues/1", Fields: map[string]any{"repo": "acme/widgets"}},
			want: "git@git.example.com:acme/widgets.git", ok: true,
		},
		{
			name: "reuses exact session SSH remote",
			item: sources.Item{URI: "https://code.example.net/acme/session-repo/issues/1", Fields: map[string]any{"repo": "acme/session-repo"}},
			want: "ssh://git@code.example.net/acme/session-repo.git", ok: true,
		},
		{
			name: "same repo on another host constructs distinct HTTPS remote",
			item: sources.Item{URI: "https://other.example.com/acme/widgets/issues/1", Fields: map[string]any{"repo": "acme/widgets"}},
			want: "https://other.example.com/acme/widgets", ok: true,
		},
		{
			name: "missing repo rejected",
			item: sources.Item{URI: "https://git.example.com/acme/widgets/issues/1"},
			ok:   false,
		},
		{
			name: "malformed repo rejected",
			item: sources.Item{URI: "https://git.example.com/acme/widgets/issues/1", Fields: map[string]any{"repo": "acme/widgets/extra"}},
			ok:   false,
		},
		{
			name: "malformed host rejected",
			item: sources.Item{URI: "not a URL", Fields: map[string]any{"repo": "acme/widgets"}},
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := m.resolveItemRemote(tt.item)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSourceItemScope(t *testing.T) {
	assert.Equal(t, "item/repo", sourceItemScope(sources.Item{Fields: map[string]any{"repo": "item/repo"}}, "picker/repo"))
	assert.Equal(t, "picker/repo", sourceItemScope(sources.Item{Fields: map[string]any{"repo": ""}}, "picker/repo"))
	assert.Equal(t, "picker/repo", sourceItemScope(sources.Item{Fields: map[string]any{"repo": 42}}, "picker/repo"))
}

func TestSourceFormSubmit_PreservesRenderedPromptTagsAndEditedInputs(t *testing.T) {
	creator := &fakeSessionCreator{}
	var fetched []sources.FetchDetailParams
	src := stubSource{
		id: "global", detail: sources.Detail{Markdown: &sources.MarkdownDetail{Content: "full detail"}}, fetchParams: &fetched,
	}
	result := sourcepicker.Result{
		Item:      sources.Item{ID: "9", URI: "https://git.example.com/acme/widgets/issues/9", Fields: map[string]any{"repo": "acme/widgets"}},
		SourceID:  "global",
		Source:    src,
		Manifest:  sources.Manifest{Capabilities: sources.Capabilities{FetchDetail: true}},
		Templates: sources.TemplateConfig{Name: "generated-{{ .ID }}", Prompt: "Work {{ .Detail }}", Tags: []string{"source", "{{ .Fields.repo }}"}},
	}
	picker := sourcepicker.New(nil, "", "", "", 80, 24)
	form := NewNewSessionForm([]workspace.DiscoveredRepo{{
		Path: "/work/chosen", Name: "chosen", Remote: "git@chosen.example.com:acme/chosen.git",
	}}, "git@chosen.example.com:acme/chosen.git", "generated-9", nil, []string{"claude", "pi"})
	form.nameInput.SetValue("edited-session")
	form.selectAgent("pi")
	form.submitted = true

	m := Model{
		cmdService: command.NewService(nil, nil, nil, nil, creator),
		modals:     NewModalCoordinator(),
		state:      stateCreatingSession,
		pendingSourceForm: &pendingSourceForm{
			result: result, remote: "https://git.example.com/acme/widgets", scope: "acme/widgets",
		},
	}
	m.modals.NewSession = form
	m.modals.SourcePicker = &picker

	model, cmd := m.updateNewSessionForm(tea.WindowSizeMsg{})
	updated := model.(Model)
	require.NotNil(t, cmd)
	assert.Nil(t, updated.pendingSourceForm)
	assert.Nil(t, updated.modals.SourcePicker)

	started, ok := cmd().(bgStreamStartedMsg)
	require.True(t, ok)
	for range started.output {
	}
	require.NoError(t, <-started.done)

	require.Len(t, creator.created, 1)
	created := creator.created[0]
	assert.Equal(t, "edited-session", created.Name)
	assert.Equal(t, "git@chosen.example.com:acme/chosen.git", created.Remote)
	assert.Equal(t, "/work/chosen", created.Source)
	assert.Equal(t, "pi", created.AgentKey)
	assert.Equal(t, "Work full detail", created.Prompt)
	assert.Equal(t, []string{"source", "acme/widgets"}, created.Tags)
	assert.True(t, created.UseBatchSpawn)
	require.Len(t, fetched, 1)
	assert.Equal(t, "acme/widgets", fetched[0].Scope, "detail fetch uses the result repo, not picker scope")
}

func TestSourceFormCancel_ReturnsToPicker(t *testing.T) {
	picker := sourcepicker.New(nil, "", "", "", 80, 24)
	form := NewNewSessionForm(nil, "", "name", nil, nil)
	form.cancelled = true
	m := Model{modals: NewModalCoordinator(), state: stateCreatingSession, pendingSourceForm: &pendingSourceForm{}}
	m.modals.NewSession = form
	m.modals.SourcePicker = &picker

	model, cmd := m.updateNewSessionForm(tea.WindowSizeMsg{})
	updated := model.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, stateSourcePicker, updated.state)
	assert.Nil(t, updated.pendingSourceForm)
	assert.Nil(t, updated.modals.NewSession)
	assert.NotNil(t, updated.modals.SourcePicker)
}

func TestFetchSourceDetail(t *testing.T) {
	ctx := context.Background()
	body := sources.Detail{Markdown: &sources.MarkdownDetail{Content: "issue body"}}
	capable := sources.Manifest{Capabilities: sources.Capabilities{FetchDetail: true}}

	t.Run("fetches when capable and item has no detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detail: body}, capable)
		got := fetchSourceDetail(ctx, result, "o/r", "")
		assert.Equal(t, body, got)
	})

	t.Run("no capability falls back to body field", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "prs", detail: body}, sources.Manifest{})
		result.Item.Fields = map[string]any{"body": "field body"}
		got := fetchSourceDetail(ctx, result, "o/r", "")
		require.NotNil(t, got.Markdown)
		assert.Equal(t, "field body", got.Markdown.Content, "sources without detail capability use the body field")
	})

	t.Run("no capability and no body field yields empty detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "prs", detail: body}, sources.Manifest{})
		got := fetchSourceDetail(ctx, result, "o/r", "")
		assert.Equal(t, sources.Detail{}, got)
	})

	t.Run("fetch failure degrades to empty detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detailErr: assert.AnError}, capable)
		got := fetchSourceDetail(ctx, result, "o/r", "")
		assert.Equal(t, sources.Detail{}, got, "detail errors must not block session creation")
	})
}

func sourcepickerResult(src sources.Source, manifest sources.Manifest) sourcepicker.Result {
	return sourcepicker.Result{
		Item:     sources.Item{ID: "1", Title: "one"},
		SourceID: src.Name(),
		Source:   src,
		Manifest: manifest,
	}
}

// fakeSessionCreator records CreateSession calls for fan-out tests.
type fakeSessionCreator struct {
	created []hive.CreateOptions
}

func (f *fakeSessionCreator) CreateSession(_ context.Context, opts hive.CreateOptions) (*session.Session, error) {
	f.created = append(f.created, opts)
	return &session.Session{ID: fmt.Sprintf("id-%d", len(f.created)), Slug: opts.Name}, nil
}

func multiResult(id, nameTemplate string) sourcepicker.Result {
	return sourcepicker.Result{
		Item:      sources.Item{ID: id, Title: "item " + id},
		SourceID:  "issues",
		Source:    stubSource{id: "issues"},
		Templates: sources.TemplateConfig{Name: nameTemplate},
	}
}

func TestCreateSourceSessions_FanOut(t *testing.T) {
	creator := &fakeSessionCreator{}
	m := Model{cmdService: command.NewService(nil, nil, nil, nil, creator)}
	out := make(chan string, 100)

	results := []sourcepicker.Result{
		multiResult("1", "session-{{ .ID }}"),
		multiResult("2", "session-{{ .ID }}"),
	}

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(), results, sourcePickerScope{}, out, &firstID, &firstName)
	require.NoError(t, err)

	require.Len(t, creator.created, 2, "every selected item spawns a session")
	assert.Equal(t, "session-1", creator.created[0].Name)
	assert.Equal(t, "session-2", creator.created[1].Name)
	assert.Equal(t, "id-1", firstID, "first created session is recorded for selection")
	assert.Equal(t, "session-1", firstName)
}

func TestCreateSourceSessions_PartialFailureContinues(t *testing.T) {
	creator := &fakeSessionCreator{}
	m := Model{cmdService: command.NewService(nil, nil, nil, nil, creator)}
	out := make(chan string, 100)

	results := []sourcepicker.Result{
		multiResult("1", "session-{{ .ID }}"),
		multiResult("2", "{{ .Nope }}"), // render failure
		multiResult("3", "session-{{ .ID }}"),
	}

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(), results, sourcePickerScope{}, out, &firstID, &firstName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 3 sessions failed (#2)",
		"summary names the failed items so the user knows what to retry")

	require.Len(t, creator.created, 2, "items after a failure must still spawn")
	assert.Equal(t, "session-1", creator.created[0].Name)
	assert.Equal(t, "session-3", creator.created[1].Name)

	close(out)
	var lines []string
	for line := range out {
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, "[2/3] failed:", "per-item failure is written to the stream (discarded while backgrounded)")
}

func TestCreateSourceSessions_SingleItemErrorPassesThrough(t *testing.T) {
	creator := &fakeSessionCreator{}
	m := Model{cmdService: command.NewService(nil, nil, nil, nil, creator)}
	out := make(chan string, 100)

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(),
		[]sourcepicker.Result{multiResult("1", "{{ .Nope }}")},
		sourcePickerScope{}, out, &firstID, &firstName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name template", "single-item batches surface the underlying error")
	assert.Empty(t, creator.created)
}

func TestDetectSourceBackend(t *testing.T) {
	t.Run("github remote", func(t *testing.T) {
		m := Model{cfg: &config.Config{}}
		assert.Equal(t, sources.BackendGithub, m.detectSourceBackend("git@github.com:o/r.git"))
	})

	t.Run("gitea host heuristic", func(t *testing.T) {
		m := Model{cfg: &config.Config{}}
		assert.Equal(t, sources.BackendGitea, m.detectSourceBackend("https://gitea.example.com/o/r"))
	})

	t.Run("config override wins", func(t *testing.T) {
		m := Model{cfg: &config.Config{Sources: config.SourcesConfig{
			Hosts: map[string]string{"git.acme.com": "gitea"},
		}}}
		assert.Equal(t, sources.BackendGitea, m.detectSourceBackend("git@git.acme.com:o/r.git"))
	})

	t.Run("empty remote defaults to github", func(t *testing.T) {
		m := Model{cfg: &config.Config{}}
		assert.Equal(t, sources.BackendGithub, m.detectSourceBackend(""))
	})
}

func TestSourcePickerScopeForSelection(t *testing.T) {
	t.Run("explicit scope arg overrides selection", func(t *testing.T) {
		m := Model{}
		sess := &session.Session{Remote: "git@github.com:other/repo.git"}
		scope := m.sourcePickerScopeForSelection(sess, []string{"issues", "explicit/scope"})
		assert.Equal(t, "explicit/scope", scope.Search)
		assert.Empty(t, scope.Remote, "explicit scope without a discovered repo has no remote")
	})

	t.Run("selected session remote derives scope", func(t *testing.T) {
		m := Model{}
		sess := &session.Session{Remote: "git@github.com:myorg/myrepo.git"}
		scope := m.sourcePickerScopeForSelection(sess, nil)
		assert.Equal(t, "myorg/myrepo", scope.Search)
		assert.Equal(t, "git@github.com:myorg/myrepo.git", scope.Remote)
	})

	t.Run("no selection yields empty scope", func(t *testing.T) {
		m := Model{}
		scope := m.sourcePickerScopeForSelection(nil, nil)
		assert.Empty(t, scope.Search)
		assert.Empty(t, scope.Remote)
	})
}
