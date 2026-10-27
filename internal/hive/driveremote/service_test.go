package driveremote_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/driveremote"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
)

const testToken = "tok-stored"

// fakeServer implements the document routes of one repository space.
type fakeServer struct {
	*httptest.Server

	mu       sync.Mutex
	docs     map[string]string
	puts     []string
	gets     []string
	tokens   []string
	tooLarge map[string]bool
	rejected bool
}

func (f *fakeServer) rejectAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejected = true
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{docs: map[string]string{}, tooLarge: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		f.writeJSON(w, driveapi.Me{Email: "dev@example.com", Policy: "dev"})
	})
	mux.HandleFunc("GET /api/v1/repos/acme/site", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var out []driveapi.Document
		for p, body := range f.docs {
			out = append(out, driveapi.Document{Path: p, SHA256: sum(body), Kind: drive.KindOf(p)})
		}
		f.writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/v1/repos/acme/site/docs/{path...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, ok := f.docs[r.PathValue("path")]
		if !ok {
			f.fail(w, http.StatusNotFound, "not_found")
			return
		}
		f.gets = append(f.gets, r.PathValue("path"))
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("PUT /api/v1/repos/acme/site/docs/{path...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.PathValue("path")
		if f.tooLarge[p] {
			f.fail(w, http.StatusRequestEntityTooLarge, "too_large")
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.docs[p] = string(body)
		f.puts = append(f.puts, p)
		f.writeJSON(w, driveapi.Document{Path: p, Kind: drive.KindOf(p), SHA256: sum(string(body))})
	})
	mux.HandleFunc("POST /api/v1/repos/acme/site/shares/{path...}", func(w http.ResponseWriter, r *http.Request) {
		var req driveapi.ShareRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.writeJSON(w, driveapi.Share{URL: "https://drive.test/s/abc?path=" + r.PathValue("path") + "&exp=" + req.ExpiresIn})
	})
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.tokens = append(f.tokens, r.Header.Get("Authorization"))
		rejected := f.rejected
		f.mu.Unlock()
		if rejected || r.Header.Get("Authorization") == "" {
			f.fail(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeServer) fail(w http.ResponseWriter, status int, kind string) {
	w.WriteHeader(status)
	f.writeJSON(w, driveapi.ErrorBody{Kind: kind, Message: kind})
}

func (f *fakeServer) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

func (f *fakeServer) getCount(p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, got := range f.gets {
		if got == p {
			n++
		}
	}
	return n
}

func (f *fakeServer) lastToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokens[len(f.tokens)-1]
}

func sum(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

type fixture struct {
	svc    *driveremote.Service
	files  *drivesvc.Service
	root   string
	creds  *credentials.MemoryStore
	server *fakeServer
	space  drive.Space
	env    map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv(credentials.EnvOverrideName("drive"), "")
	root := t.TempDir()
	cfg := &config.Config{Context: config.ContextConfig{BaseDir: root, SymlinkName: ".hive"}}
	files := drivesvc.NewService(zerolog.Nop(), cfg, nil, testbus.New(t).EventBus)
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	space, err := drive.RepositorySpace("acme", "site")
	require.NoError(t, err)
	f := &fixture{
		files:  files,
		root:   root,
		creds:  credentials.NewMemoryStore(),
		server: newFakeServer(t),
		space:  space,
		env:    map[string]string{},
	}
	f.svc = driveremote.New(zerolog.Nop(), files, store.NewDriveUploadStore(database), f.creds, func(k string) string { return f.env[k] })
	f.signIn(t, testToken)
	return f
}

func (f *fixture) host(t *testing.T) string {
	t.Helper()
	host, err := driveapi.Host(f.server.URL)
	require.NoError(t, err)
	return host
}

func (f *fixture) signIn(t *testing.T, token string) {
	t.Helper()
	require.NoError(t, f.creds.Set(credentials.Ref{Provider: "drive", Account: f.host(t)}, token))
}

func (f *fixture) writeLocal(t *testing.T, path, body string) {
	t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(f.space.Key), filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func (f *fixture) readLocal(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(f.space.Key), filepath.FromSlash(path)))
	require.NoError(t, err)
	return string(data)
}

func actions[T any](results []T, path func(T) string, action func(T) driveremote.Action) map[string]driveremote.Action {
	out := map[string]driveremote.Action{}
	for _, r := range results {
		out[path(r)] = action(r)
	}
	return out
}

func pushActions(results []driveremote.PushResult) map[string]driveremote.Action {
	return actions(results, func(r driveremote.PushResult) string { return r.Path }, func(r driveremote.PushResult) driveremote.Action { return r.Action })
}

func pullActions(results []driveremote.PullResult) map[string]driveremote.Action {
	return actions(results, func(r driveremote.PullResult) string { return r.Path }, func(r driveremote.PullResult) driveremote.Action { return r.Action })
}

func TestPushSkipsUnchangedUnlessForced(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "plans/ship.md", "# Ship\n")
	f.writeLocal(t, "notes.txt", "not pushable")

	results, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]driveremote.Action{"plans/ship.md": driveremote.ActionUploaded}, pushActions(results))
	assert.Equal(t, sum("# Ship\n"), results[0].SHA256)

	results, err = f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionUnchanged, results[0].Action)
	assert.Equal(t, 1, f.server.putCount())

	results, err = f.svc.Push(t.Context(), f.server.URL, f.space, nil, true)
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionUploaded, results[0].Action)
	assert.Equal(t, 2, f.server.putCount())

	f.writeLocal(t, "plans/ship.md", "# Ship it\n")
	results, err = f.svc.Push(t.Context(), f.server.URL, f.space, []string{"plans"}, false)
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionUploaded, results[0].Action, "a folder path pushes what is under it")
}

func TestPushRecordsTooLargeAndContinues(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "a-big.md", "big")
	f.writeLocal(t, "b-small.md", "small")
	f.server.tooLarge["a-big.md"] = true

	results, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]driveremote.Action{
		"a-big.md":   driveremote.ActionFailed,
		"b-small.md": driveremote.ActionUploaded,
	}, pushActions(results))
	assert.Contains(t, results[0].Error, "too_large")
}

func TestPushReportsUnpushablePaths(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "notes.txt", "x")

	results, err := f.svc.Push(t.Context(), f.server.URL, f.space, []string{"notes.txt", "missing.md"}, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]driveremote.Action{
		"notes.txt":  driveremote.ActionFailed,
		"missing.md": driveremote.ActionFailed,
	}, pushActions(results))
}

func TestPushStopsOnRejectedCredentials(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "a.md", "a")
	f.writeLocal(t, "b.md", "b")
	f.server.rejectAll()

	results, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.Error(t, err)
	var apiErr *driveapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Len(t, results, 1, "the second document is not attempted")
}

func TestPushWithoutLoginExplainsHowToSignIn(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.creds.Delete(credentials.Ref{Provider: "drive", Account: f.host(t)}))

	_, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.ErrorIs(t, err, driveremote.ErrNotSignedIn)
}

func TestPullCreatesAndSkipsIdentical(t *testing.T) {
	f := newFixture(t)
	f.server.docs["plans/new.md"] = "# New\n"
	f.server.docs["same.md"] = "same"
	f.writeLocal(t, "same.md", "same")

	results, err := f.svc.Pull(t.Context(), f.server.URL, f.space, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]driveremote.Action{
		"plans/new.md": driveremote.ActionCreated,
		"same.md":      driveremote.ActionUnchanged,
	}, pullActions(results))
	assert.Equal(t, "# New\n", f.readLocal(t, "plans/new.md"))
	assert.Equal(t, 0, f.server.getCount("same.md"), "a matching local copy needs no download")
}

func TestPullKeepsDifferentLocalFileAndNumbersCopies(t *testing.T) {
	f := newFixture(t)
	f.server.docs["plan.md"] = "remote"
	f.writeLocal(t, "plan.md", "local")

	results, err := f.svc.Pull(t.Context(), f.server.URL, f.space, nil)
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionConflict, results[0].Action)
	assert.Equal(t, "plan-1.md", results[0].LocalPath)

	results, err = f.svc.Pull(t.Context(), f.server.URL, f.space, []string{"plan.md"})
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionUnchanged, results[0].Action, "the -1 copy already holds this content")
	assert.NoFileExists(t, filepath.Join(f.root, "acme", "site", "plan-2.md"))

	f.server.docs["plan.md"] = "remote v2"
	results, err = f.svc.Pull(t.Context(), f.server.URL, f.space, []string{"plan.md"})
	require.NoError(t, err)
	assert.Equal(t, "plan-2.md", results[0].LocalPath, "new content takes the next free number")

	assert.Equal(t, "local", f.readLocal(t, "plan.md"))
	assert.Equal(t, "remote", f.readLocal(t, "plan-1.md"))
	assert.Equal(t, "remote v2", f.readLocal(t, "plan-2.md"))
}

func TestPullTreatsCaseOnlyDifferencesAsCollisions(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "plan.md", "local")
	f.writeLocal(t, "same.md", "same")
	f.server.docs["Same.md"] = "same"
	f.server.docs["Plan.md"] = "remote upper"
	f.server.docs["Notes.md"] = "remote notes"
	f.server.docs["notes.md"] = "remote notes lower"

	results, err := f.svc.Pull(t.Context(), f.server.URL, f.space, nil)
	require.NoError(t, err)
	byPath := map[string]driveremote.PullResult{}
	for _, r := range results {
		byPath[r.Path] = r
	}
	assert.Equal(t, "Plan-1.md", byPath["Plan.md"].LocalPath, "plan.md is local, so Plan.md must not replace it")
	assert.Equal(t, driveremote.ActionCreated, byPath["Notes.md"].Action)
	assert.Equal(t, "notes-1.md", byPath["notes.md"].LocalPath, "two remote names that fold together do not overwrite each other")
	assert.Equal(t, driveremote.ActionUnchanged, byPath["Same.md"].Action, "a case-only variant with the same content is not copied")
	assert.NoFileExists(t, filepath.Join(f.root, "acme", "site", "Same-1.md"))
	assert.Equal(t, "local", f.readLocal(t, "plan.md"))
	assert.Equal(t, "remote notes", f.readLocal(t, "Notes.md"))
}

func TestPullReportsMissingRemotePath(t *testing.T) {
	f := newFixture(t)
	results, err := f.svc.Pull(t.Context(), f.server.URL, f.space, []string{"nope.md"})
	require.NoError(t, err)
	assert.Equal(t, driveremote.ActionFailed, results[0].Action)
}

func TestLoginStoresOnlyAcceptedTokens(t *testing.T) {
	f := newFixture(t)
	creds := credentials.NewMemoryStore()
	svc := driveremote.New(zerolog.Nop(), f.files, nil, creds, func(string) string { return "" })

	me, err := svc.Login(t.Context(), f.server.URL, "fresh-token")
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", me.Email)

	got, err := creds.Get(credentials.Ref{Provider: "drive", Account: f.host(t)})
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", got)

	f.server.rejectAll()
	rejected := credentials.NewMemoryStore()
	_, err = driveremote.New(zerolog.Nop(), f.files, nil, rejected, func(string) string { return "" }).Login(t.Context(), f.server.URL, "bad")
	require.Error(t, err)
	refs, err := rejected.List()
	require.NoError(t, err)
	assert.Empty(t, refs, "a rejected token is never stored")
}

func TestEnvTokenWinsOverStoredToken(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "a.md", "a")

	_, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+testToken, f.server.lastToken())

	t.Setenv(credentials.EnvOverrideName("drive"), "tok-env")
	_, err = f.svc.Push(t.Context(), f.server.URL, f.space, nil, true)
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-env", f.server.lastToken())
}

func TestDefaultServerOrder(t *testing.T) {
	f := newFixture(t)

	got, err := f.svc.DefaultServer("flag.example")
	require.NoError(t, err)
	assert.Equal(t, "flag.example", got, "the flag wins")

	f.env[driveremote.EnvServer] = "env.example"
	got, err = f.svc.DefaultServer("")
	require.NoError(t, err)
	assert.Equal(t, "env.example", got, "the environment beats a stored login")

	delete(f.env, driveremote.EnvServer)
	got, err = f.svc.DefaultServer("")
	require.NoError(t, err)
	assert.Equal(t, f.host(t), got, "the one stored login")

	require.NoError(t, f.creds.Set(credentials.Ref{Provider: "drive", Account: "other.example"}, "x"))
	_, err = f.svc.DefaultServer("")
	require.Error(t, err, "two logins are ambiguous")
	require.NotErrorIs(t, err, driveremote.ErrNoServer)

	empty := driveremote.New(zerolog.Nop(), f.files, nil, credentials.NewMemoryStore(), func(string) string { return "" })
	_, err = empty.DefaultServer("")
	require.ErrorIs(t, err, driveremote.ErrNoServer)
}

func TestStatusComparesLocalFilesToRecords(t *testing.T) {
	f := newFixture(t)
	f.writeLocal(t, "kept.md", "kept")
	f.writeLocal(t, "edited.md", "v1")
	f.writeLocal(t, "gone.md", "gone")
	f.writeLocal(t, "unpushed.txt", "x")
	_, err := f.svc.Push(t.Context(), f.server.URL, f.space, nil, false)
	require.NoError(t, err)

	f.writeLocal(t, "edited.md", "v2")
	require.NoError(t, os.Remove(filepath.Join(f.root, "acme", "site", "gone.md")))
	f.writeLocal(t, "later.md", "never pushed")

	status, err := f.svc.Status(t.Context(), f.server.URL, f.space)
	require.NoError(t, err)
	assert.Equal(t, map[string]driveremote.UploadStatus{
		"kept.md":   driveremote.StatusUploaded,
		"edited.md": driveremote.StatusChanged,
	}, status)
}

func TestShareReturnsServerLink(t *testing.T) {
	f := newFixture(t)
	share, err := f.svc.Share(t.Context(), f.server.URL, f.space, "plans/ship.md", "7d")
	require.NoError(t, err)
	assert.Equal(t, "https://drive.test/s/abc?path=plans/ship.md&exp=7d", share.URL)
}
