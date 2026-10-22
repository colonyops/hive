package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/sources"
	"github.com/colonyops/hive/cmd/hive/internal/tui/command"
	"github.com/colonyops/hive/cmd/hive/internal/tui/sourcepicker"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// stubSource is a minimal sources.Source for registry-backed wiring tests.
type stubSource struct {
	id        string
	detail    sources.Detail
	detailErr error
}

func (s stubSource) Name() string                   { return s.id }
func (s stubSource) Available(context.Context) bool { return true }
func (s stubSource) Initialize(context.Context) (sources.Manifest, error) {
	return sources.Manifest{ID: s.id}, nil
}

func (s stubSource) Search(context.Context, sources.SearchParams) (sources.SearchResult, error) {
	return sources.SearchResult{}, nil
}

func (s stubSource) FetchDetail(context.Context, sources.FetchDetailParams) (sources.Detail, error) {
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

func TestFetchSourceDetail(t *testing.T) {
	ctx := context.Background()
	body := sources.Detail{Markdown: &sources.MarkdownDetail{Content: "issue body"}}
	capable := sources.Manifest{Capabilities: sources.Capabilities{FetchDetail: true}}

	t.Run("fetches when capable and item has no detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detail: body}, capable)
		got := fetchSourceDetail(ctx, zerolog.Nop(), result, "o/r", "")
		assert.Equal(t, body, got)
	})

	t.Run("no capability falls back to body field", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "prs", detail: body}, sources.Manifest{})
		result.Item.Fields = map[string]any{"body": "field body"}
		got := fetchSourceDetail(ctx, zerolog.Nop(), result, "o/r", "")
		require.NotNil(t, got.Markdown)
		assert.Equal(t, "field body", got.Markdown.Content, "sources without detail capability use the body field")
	})

	t.Run("no capability and no body field yields empty detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "prs", detail: body}, sources.Manifest{})
		got := fetchSourceDetail(ctx, zerolog.Nop(), result, "o/r", "")
		assert.Equal(t, sources.Detail{}, got)
	})

	t.Run("fetch failure degrades to empty detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detailErr: assert.AnError}, capable)
		got := fetchSourceDetail(ctx, zerolog.Nop(), result, "o/r", "")
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
	created []sessionsvc.CreateOptions
}

func (f *fakeSessionCreator) CreateSession(_ context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	f.created = append(f.created, opts)
	return &session.Session{ID: fmt.Sprintf("id-%d", len(f.created)), Slug: opts.Name}, nil
}

type fakeLaunchResolver struct {
	generated map[string]string
	preferred map[string]string
	resolved  []string
	source    string
	err       error
}

func (f *fakeLaunchResolver) GeneratedRemote(repo, itemURL string) string {
	return f.generated[repo+"|"+itemURL]
}

func (f *fakeLaunchResolver) PreferredRemote(repo, itemURL string) string {
	return f.preferred[repo+"|"+itemURL]
}

func (f *fakeLaunchResolver) ResolveSessionLaunchRepository(_ context.Context, remote string) (sessionsvc.LaunchRepository, error) {
	f.resolved = append(f.resolved, remote)
	return sessionsvc.LaunchRepository{Remote: remote, Source: f.source}, f.err
}

func newCreateModel(creator *fakeSessionCreator, resolver *fakeLaunchResolver) Model {
	return Model{cmdService: command.NewService(nil, nil, nil, nil, nil, creator), launches: resolver}
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
	m := newCreateModel(creator, &fakeLaunchResolver{})
	out := make(chan string, 100)

	results := []sourcepicker.Result{
		multiResult("1", "session-{{ .ID }}"),
		multiResult("2", "session-{{ .ID }}"),
	}

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(), results, sourcePickerScope{Remote: "https://github.com/acme/site.git"}, out, &firstID, &firstName)
	require.NoError(t, err)

	require.Len(t, creator.created, 2, "every selected item spawns a session")
	assert.Equal(t, "session-1", creator.created[0].Name)
	assert.Equal(t, "session-2", creator.created[1].Name)
	assert.Equal(t, "1", creator.created[0].CollisionSuffix, "a taken name gets the item id")
	assert.Equal(t, "id-1", firstID, "first created session is recorded for selection")
	assert.Equal(t, "session-1", firstName)
}

func TestCreateSourceSessions_PartialFailureContinues(t *testing.T) {
	creator := &fakeSessionCreator{}
	m := newCreateModel(creator, &fakeLaunchResolver{})
	out := make(chan string, 100)

	results := []sourcepicker.Result{
		multiResult("1", "session-{{ .ID }}"),
		multiResult("2", "{{ .Nope }}"), // render failure
		multiResult("3", "session-{{ .ID }}"),
	}

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(), results, sourcePickerScope{Remote: "https://github.com/acme/site.git"}, out, &firstID, &firstName)
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
	m := newCreateModel(creator, &fakeLaunchResolver{})
	out := make(chan string, 100)

	var firstID, firstName string
	err := m.createSourceSessions(context.Background(),
		[]sourcepicker.Result{multiResult("1", "{{ .Nope }}")},
		sourcePickerScope{Remote: "https://github.com/acme/site.git"}, out, &firstID, &firstName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name template", "single-item batches surface the underlying error")
	assert.Empty(t, creator.created)
}

func TestCreateSourceSession_ResolvesThePreferredRemote(t *testing.T) {
	creator := &fakeSessionCreator{}
	resolver := &fakeLaunchResolver{
		preferred: map[string]string{"acme/site|https://github.com/acme/site/issues/7": "git@github.com:acme/site.git"},
		source:    "/checkouts/site",
	}
	m := newCreateModel(creator, resolver)
	result := multiResult("7", "session-{{ .ID }}")
	result.Item.Fields = map[string]any{"url": "https://github.com/acme/site/issues/7"}

	_, _, err := m.createSourceSession(t.Context(), result,
		sourcePickerScope{Search: "acme/site", Remote: "https://github.com/acme/site.git"}, make(chan string, 10))
	require.NoError(t, err)

	assert.Equal(t, []string{"git@github.com:acme/site.git"}, resolver.resolved)
	require.Len(t, creator.created, 1)
	assert.Equal(t, "git@github.com:acme/site.git", creator.created[0].Remote)
	assert.Equal(t, "/checkouts/site", creator.created[0].Source, "the resolved checkout is the copy source")
}

func TestCreateSourceSession_NoPreferenceKeepsTheScopeRemote(t *testing.T) {
	creator := &fakeSessionCreator{}
	resolver := &fakeLaunchResolver{
		generated: map[string]string{"acme/site|https://github.com/acme/site/issues/7": "https://github.com/acme/site.git"},
	}
	m := newCreateModel(creator, resolver)
	result := multiResult("7", "session-{{ .ID }}")
	result.Item.Fields = map[string]any{"url": "https://github.com/acme/site/issues/7"}

	_, _, err := m.createSourceSession(t.Context(), result,
		sourcePickerScope{Search: "acme/site", Remote: "git@github-work:acme/site.git"}, make(chan string, 10))
	require.NoError(t, err)

	assert.Equal(t, []string{"git@github-work:acme/site.git"}, resolver.resolved, "an SSH host alias must survive")
}

func TestCreateSourceSession_GeneratesWithoutAScopeRemote(t *testing.T) {
	creator := &fakeSessionCreator{}
	resolver := &fakeLaunchResolver{
		generated: map[string]string{"acme/site|https://github.com/acme/site/issues/7": "https://github.com/acme/site.git"},
	}
	m := newCreateModel(creator, resolver)
	result := multiResult("7", "session-{{ .ID }}")
	result.Item.Fields = map[string]any{"url": "https://github.com/acme/site/issues/7"}

	_, _, err := m.createSourceSession(t.Context(), result, sourcePickerScope{Search: "acme/site"}, make(chan string, 10))
	require.NoError(t, err)

	assert.Equal(t, []string{"https://github.com/acme/site.git"}, resolver.resolved)
}

func TestCreateSourceSession_ScopeSourceWinsOverResolvedSource(t *testing.T) {
	creator := &fakeSessionCreator{}
	resolver := &fakeLaunchResolver{source: "/checkouts/resolved"}
	m := newCreateModel(creator, resolver)

	_, _, err := m.createSourceSession(t.Context(), multiResult("1", "s-{{ .ID }}"),
		sourcePickerScope{Remote: "https://github.com/acme/site.git", Source: "/checkouts/scope"}, make(chan string, 10))
	require.NoError(t, err)

	assert.Equal(t, []string{"https://github.com/acme/site.git"}, resolver.resolved, "the scope remote is the fallback request")
	assert.Equal(t, "/checkouts/scope", creator.created[0].Source)
}

func TestCreateSourceSession_NoRemoteIsAnErrorNotTheCwd(t *testing.T) {
	creator := &fakeSessionCreator{}
	resolver := &fakeLaunchResolver{}
	m := newCreateModel(creator, resolver)

	_, _, err := m.createSourceSession(t.Context(), multiResult("1", "s-{{ .ID }}"),
		sourcePickerScope{Search: "acme/site"}, make(chan string, 10))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "acme/site")
	assert.Empty(t, resolver.resolved)
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
