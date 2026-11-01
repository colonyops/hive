package app

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/pathutil"
)

// This file is what window.hive reaches. An applet is a page the user asked
// for on their own machine, so these calls are deliberately unrestricted: any
// command, any path (ADR applets-are-web-pages-with-full-access-to-the-machine).

const (
	appletStorageFile = ".localstorage.json"
	// spawnReadWait is how long a read holds open with nothing to report, so a quiet
	// process costs a page one round trip every few seconds rather than a busy loop.
	spawnReadWait = 10 * time.Second
	// maxSpawnBuffer is what a process may print between reads. Past it the oldest
	// output goes, because a page that stopped reading must not grow this forever.
	maxSpawnBuffer = 4 << 20
)

// AppletExec is one command a page asks for. Cwd is relative to the applet's own
// default directory. TimeoutMillis zero means appletActionTimeout.
type AppletExec struct {
	Command       string            `json:"command"`
	SessionSlug   string            `json:"sessionSlug"`
	Cwd           string            `json:"cwd"`
	Env           map[string]string `json:"env"`
	Stdin         string            `json:"stdin"`
	TimeoutMillis int64             `json:"timeoutMillis"`
}

// AppletExecResult reports the command's outcome. A non-zero exit is a result the
// page reads, not an error, as it would be in a script.
type AppletExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
}

// AppletContext is what a page can ask about where it is running.
type AppletContext struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Dir         string `json:"dir"`
	Cwd         string `json:"cwd"`
	SessionSlug string `json:"sessionSlug"`
}

// AppletAsset is one file of an applet, ready to serve.
type AppletAsset struct {
	Name        string
	ContentType string
	Body        []byte
	// HoldsToken marks a response carrying the bridge token, which must never be
	// readable cross-origin.
	HoldsToken bool
}

func (s *AppletsService) page(id string) (applets.Applet, error) {
	applet, err := s.applet(id)
	if err != nil {
		return applets.Applet{}, err
	}
	if !s.pages {
		return applets.Applet{}, Errorf(KindUnavailable, appletPagesUnavailable)
	}
	if s.isDisabled(id) {
		return applets.Applet{}, Errorf(KindUnavailable, "%s is turned off. Turn it on in Settings", applet.Title)
	}
	return applet, nil
}

// Context answers hive.context().
func (s *AppletsService) Context(ctx context.Context, id, sessionSlug string) (AppletContext, error) {
	applet, err := s.page(id)
	if err != nil {
		return AppletContext{}, err
	}
	cwd, err := s.dir(ctx, applet, sessionSlug)
	if err != nil {
		return AppletContext{}, err
	}
	return AppletContext{ID: applet.ID, Title: applet.Title, Dir: applet.Dir, Cwd: cwd, SessionSlug: sessionSlug}, nil
}

// BridgeToken is the secret a page proves itself with on the bridge. The frame can
// navigate away from the applet and keep its window, so the window alone does not say
// who is asking. Only pages served by Asset hold the token.
func (s *AppletsService) BridgeToken(id string) (string, error) {
	if _, err := s.page(id); err != nil {
		return "", err
	}
	s.tokensMu.Lock()
	defer s.tokensMu.Unlock()
	if token, ok := s.tokens[id]; ok {
		return token, nil
	}
	token := rand.Text()
	s.tokens[id] = token
	return token, nil
}

// Asset reads one file of an applet. An HTML file comes back with the bridge in its
// head, so window.hive exists before any of the page's own scripts run.
func (s *AppletsService) Asset(id, name string) (AppletAsset, error) {
	applet, err := s.page(id)
	if err != nil {
		return AppletAsset{}, err
	}
	if name == "" {
		name = applet.Entry
	}
	if !filepath.IsLocal(name) || hasHiddenSegment(name) {
		return AppletAsset{}, Errorf(KindNotFound, "no file %q in applet %q", name, id)
	}
	var body []byte
	if applet.Builtin {
		body, err = applets.ReadBuiltin(id, name)
	} else {
		body, err = os.ReadFile(filepath.Join(applet.Dir, filepath.FromSlash(name)))
	}
	if err != nil {
		return AppletAsset{}, Wrap(err, KindNotFound, "reading %q of applet %q", name, id)
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	if strings.HasPrefix(contentType, "text/html") {
		token, err := s.BridgeToken(id)
		if err != nil {
			return AppletAsset{}, err
		}
		body = injectBridge(body, token, readAppletStorage(applet.Dir))
		return AppletAsset{Name: name, ContentType: contentType, Body: body, HoldsToken: true}, nil
	}
	return AppletAsset{Name: name, ContentType: contentType, Body: body}, nil
}

// hasHiddenSegment refuses dotfiles, which is where a page's saved localStorage lives:
// the page already receives it inline, and nothing has a reason to fetch it.
func hasHiddenSegment(name string) bool {
	return slices.ContainsFunc(strings.Split(filepath.ToSlash(name), "/"), func(segment string) bool {
		return strings.HasPrefix(segment, ".")
	})
}

func injectBridge(page []byte, token string, storage []byte) []byte {
	// The JSON sits inside a script element, so a stored "</script>" must not end it.
	var safe bytes.Buffer
	json.HTMLEscape(&safe, storage)
	script := []byte(`<script>window.__hiveToken="` + token + `";window.__hiveStorage=` + safe.String() + ";" + applets.BridgeScript() + "</script>")
	at := bridgeInsertAt(page)
	return append(append(append([]byte{}, page[:at]...), script...), page[at:]...)
}

// bridgeInsertAt finds the first place a script can go that is ahead of the page's own
// and does not end the prologue: anything before a doctype would put the page in quirks
// mode. It reads the start of the document as the parser does, so a <header> or a <head>
// inside a comment is not mistaken for the head.
func bridgeInsertAt(page []byte) int {
	at := 0
	for {
		rest := bytes.TrimPrefix(page[at:], []byte("\xef\xbb\xbf"))
		rest = bytes.TrimLeft(rest, " \t\r\n\f")
		start := len(page) - len(rest)
		end := -1
		switch {
		case bytes.HasPrefix(rest, []byte("<!--")):
			if closes := bytes.Index(rest, []byte("-->")); closes >= 0 {
				end = closes + len("-->")
			}
		case hasTag(rest, "<!doctype"), hasTag(rest, "<html"):
			end = tagEnd(rest)
		case hasTag(rest, "<head"):
			if tag := tagEnd(rest); tag >= 0 {
				return start + tag
			}
		}
		if end < 0 {
			return at
		}
		at = start + end
	}
}

func hasTag(rest []byte, name string) bool {
	if len(rest) <= len(name) || !strings.EqualFold(string(rest[:len(name)]), name) {
		return false
	}
	return strings.IndexByte(" \t\r\n\f/>", rest[len(name)]) >= 0
}

// tagEnd returns the offset just past the tag's ">", which an attribute value can contain.
func tagEnd(rest []byte) int {
	var quote byte
	for i, c := range rest {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i + 1
		}
	}
	return -1
}

func readAppletStorage(dir string) []byte {
	raw, err := os.ReadFile(filepath.Join(dir, appletStorageFile))
	if err != nil || !json.Valid(raw) {
		return []byte("{}")
	}
	return raw
}

func (s *AppletsService) resolveFile(id, path string) (string, error) {
	applet, err := s.page(id)
	if err != nil {
		return "", err
	}
	expanded, err := pathutil.ExpandHomeE(path)
	if err != nil {
		return "", Wrap(err, KindInternal, "expanding %q", path)
	}
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	return filepath.Join(applet.Dir, expanded), nil
}

// ReadFile answers hive.readFile. A relative path is relative to the applet's
// directory.
func (s *AppletsService) ReadFile(id, path string) (string, error) {
	resolved, err := s.resolveFile(id, path)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return "", Wrap(err, KindNotFound, "reading %q", path)
	}
	return string(raw), nil
}

// WriteFile answers hive.writeFile, creating parent directories as a script would
// otherwise have to.
func (s *AppletsService) WriteFile(id, path, content string) error {
	resolved, err := s.resolveFile(id, path)
	if err != nil {
		return err
	}
	return writeAppletFile(resolved, path, content)
}

func writeAppletFile(resolved, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return Wrap(err, KindInternal, "creating the directory of %q", path)
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil { //nolint:gosec // the page asked for this file on the user's behalf
		return Wrap(err, KindInternal, "writing %q", path)
	}
	return nil
}

// SaveStorage persists the page's localStorage beside it.
func (s *AppletsService) SaveStorage(id, encoded string) error {
	if !json.Valid([]byte(encoded)) {
		return Errorf(KindInvalid, "applet storage is not JSON")
	}
	// Not through page: a page saves as it closes, which is also what turning it off does.
	applet, err := s.applet(id)
	if err != nil {
		return err
	}
	if !s.pages {
		return Errorf(KindUnavailable, appletPagesUnavailable)
	}
	return writeAppletFile(filepath.Join(applet.Dir, appletStorageFile), appletStorageFile, encoded)
}

// command builds the process. Its context is detached because RunShellProcess owns
// stopping it, with a SIGTERM and a grace period. A cancellable context here would
// also fire the standard library's immediate SIGKILL, so traps would never run.
func (s *AppletsService) command(ctx context.Context, id string, req AppletExec) (*exec.Cmd, error) {
	applet, err := s.page(id)
	if err != nil {
		return nil, err
	}
	dir, err := s.dir(ctx, applet, req.SessionSlug)
	if err != nil {
		return nil, err
	}
	if req.Cwd != "" {
		cwd, err := pathutil.ExpandHomeE(req.Cwd)
		if err != nil {
			return nil, Wrap(err, KindInternal, "expanding %q", req.Cwd)
		}
		if filepath.IsAbs(cwd) {
			dir = cwd
		} else {
			dir = filepath.Join(dir, cwd)
		}
	}
	proc := exec.CommandContext(context.WithoutCancel(ctx), "sh", "-c", req.Command)
	proc.Dir = dir
	proc.Env = append(s.environ(ctx), appletEnv(applet)...)
	for _, key := range slices.Sorted(maps.Keys(req.Env)) {
		proc.Env = append(proc.Env, key+"="+req.Env[key])
	}
	return proc, nil
}

func exitResult(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode(), nil
	}
	return -1, err
}

// Exec answers hive.exec.
func (s *AppletsService) Exec(ctx context.Context, id string, req AppletExec) (AppletExecResult, error) {
	proc, err := s.command(ctx, id, req)
	if err != nil {
		return AppletExecResult{}, err
	}
	timeout := appletActionTimeout
	if req.TimeoutMillis > 0 {
		timeout = time.Duration(req.TimeoutMillis) * time.Millisecond
	}
	runCtx, stop := context.WithTimeout(ctx, timeout)
	defer stop()
	defer s.spawns.trackExec(id, stop)()
	if s.isDisabled(id) {
		return AppletExecResult{}, Errorf(KindUnavailable, "this applet was turned off")
	}

	stdout := &executil.HeadWriter{Max: maxAppletStdout}
	stderr := &executil.HeadWriter{Max: maxAppletStdout}
	proc.Stdout, proc.Stderr = stdout, stderr
	proc.Stdin = strings.NewReader(req.Stdin)
	code, err := exitResult(dispatch.RunShellProcess(runCtx, proc))
	if err != nil {
		return AppletExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}, Wrap(err, KindUnavailable, "running %q", req.Command)
	}
	return AppletExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}, nil
}

// AppletOutput is what a spawned process has done since the last read. Done is set
// once, on the read that drains the end of its output.
type AppletOutput struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Done   bool   `json:"done"`
	Code   int    `json:"code"`
}

type spawnTable struct {
	mu   sync.Mutex
	next int
	live map[string]*appletSpawn
	// execs holds what a running hive.exec needs to be stopped, since its page has no
	// handle for it.
	execs map[int]runningExec
}

type runningExec struct {
	applet string
	cancel context.CancelFunc
}

func (t *spawnTable) trackExec(applet string, cancel context.CancelFunc) (untrack func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.execs == nil {
		t.execs = map[int]runningExec{}
	}
	t.next++
	key := t.next
	t.execs[key] = runningExec{applet, cancel}
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		delete(t.execs, key)
	}
}

// stopApplet ends everything an applet has running, whichever frame started it.
func (t *spawnTable) stopApplet(applet string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, spawn := range t.live {
		if spawn.applet == applet {
			spawn.cancel()
		}
	}
	for _, running := range t.execs {
		if running.applet == applet {
			running.cancel()
		}
	}
}

type appletSpawn struct {
	applet  string
	command string
	started time.Time
	cancel  context.CancelFunc
	stdin   io.WriteCloser

	mu     sync.Mutex
	stdout strings.Builder
	stderr strings.Builder
	done   bool
	code   int
	wake   chan struct{}
}

func (p *appletSpawn) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

type spawnStream struct {
	spawn  *appletSpawn
	target *strings.Builder
}

func (w spawnStream) Write(data []byte) (int, error) {
	w.spawn.mu.Lock()
	if w.target.Len()+len(data) > maxSpawnBuffer {
		kept := w.target.String()
		w.target.Reset()
		w.target.WriteString(kept[min(len(kept), len(data)):])
	}
	w.target.Write(data)
	w.spawn.mu.Unlock()
	w.spawn.notify()
	return len(data), nil
}

// Spawn answers hive.spawn: a process that outlives the call, read with SpawnRead
// until Done. It ends when the applet closes, or when the app does.
func (s *AppletsService) Spawn(ctx context.Context, id string, req AppletExec) (string, error) {
	proc, err := s.command(ctx, id, req)
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	spawn := &appletSpawn{applet: id, command: req.Command, started: time.Now(), cancel: cancel, wake: make(chan struct{}, 1)}
	proc.Stdout = spawnStream{spawn, &spawn.stdout}
	proc.Stderr = spawnStream{spawn, &spawn.stderr}
	stdin, err := proc.StdinPipe()
	if err != nil {
		cancel()
		return "", Wrap(err, KindInternal, "opening stdin for %q", req.Command)
	}
	spawn.stdin = stdin

	s.spawns.mu.Lock()
	if s.spawns.live == nil {
		s.spawns.live = map[string]*appletSpawn{}
	}
	s.spawns.next++
	handle := fmt.Sprintf("p%d", s.spawns.next)
	s.spawns.live[handle] = spawn
	s.spawns.mu.Unlock()
	if s.isDisabled(id) {
		cancel()
	}

	go func() {
		code, err := exitResult(dispatch.RunShellProcess(runCtx, proc))
		spawn.mu.Lock()
		// Being stopped is what the page asked for, so it is not output to report.
		if err != nil && !errors.Is(err, context.Canceled) {
			spawn.stderr.WriteString(err.Error())
		}
		spawn.code, spawn.done = code, true
		spawn.mu.Unlock()
		spawn.notify()
	}()
	return handle, nil
}

func (s *AppletsService) spawn(handle string) (*appletSpawn, error) {
	s.spawns.mu.Lock()
	defer s.spawns.mu.Unlock()
	spawn, ok := s.spawns.live[handle]
	if !ok {
		return nil, Errorf(KindNotFound, "no process %q", handle)
	}
	return spawn, nil
}

// SpawnRead returns what the process has printed, waiting a while if it has nothing
// to say yet.
func (s *AppletsService) SpawnRead(ctx context.Context, handle string) (AppletOutput, error) {
	spawn, err := s.spawn(handle)
	if err != nil {
		return AppletOutput{}, err
	}
	timer := time.NewTimer(spawnReadWait)
	defer timer.Stop()
	for {
		if out, ready := spawn.drain(); ready {
			if out.Done {
				s.spawns.mu.Lock()
				delete(s.spawns.live, handle)
				s.spawns.mu.Unlock()
				spawn.cancel()
			}
			return out, nil
		}
		select {
		case <-spawn.wake:
		case <-timer.C:
			return AppletOutput{}, nil
		case <-ctx.Done():
			return AppletOutput{}, ctx.Err()
		}
	}
}

func (p *appletSpawn) drain() (AppletOutput, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := AppletOutput{Stdout: takeWholeRunes(&p.stdout, p.done), Stderr: takeWholeRunes(&p.stderr, p.done), Done: p.done, Code: p.code}
	return out, out.Done || out.Stdout != "" || out.Stderr != ""
}

// takeWholeRunes empties the buffer but for a character the process has only half
// written, because output crosses to the page as a JSON string, where the two halves
// would each become a replacement character.
func takeWholeRunes(buffer *strings.Builder, final bool) string {
	text := buffer.String()
	buffer.Reset()
	if final {
		return text
	}
	whole := len(text)
	for start := len(text) - 1; start >= 0 && start >= len(text)-utf8.UTFMax; start-- {
		if utf8.RuneStart(text[start]) {
			if !utf8.FullRuneInString(text[start:]) {
				whole = start
			}
			break
		}
	}
	buffer.WriteString(text[whole:])
	return text[:whole]
}

// SpawnWrite sends text to the process's stdin.
func (s *AppletsService) SpawnWrite(handle, data string) error {
	spawn, err := s.spawn(handle)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(spawn.stdin, data); err != nil {
		return Wrap(err, KindUnavailable, "writing to process %q", handle)
	}
	return nil
}

// SpawnKill stops the process. Its end still arrives through SpawnRead.
func (s *AppletsService) SpawnKill(handle string) error {
	spawn, err := s.spawn(handle)
	if err != nil {
		return err
	}
	spawn.cancel()
	return nil
}

// AppletProcess is a process an applet has running, for Settings to show.
type AppletProcess struct {
	Handle        string `json:"handle"`
	Applet        string `json:"applet"`
	Command       string `json:"command"`
	StartedMillis int64  `json:"startedMillis"`
}

// Processes lists what applets have running, oldest first.
func (s *AppletsService) Processes() []AppletProcess {
	s.spawns.mu.Lock()
	defer s.spawns.mu.Unlock()
	out := make([]AppletProcess, 0, len(s.spawns.live))
	for handle, spawn := range s.spawns.live {
		spawn.mu.Lock()
		done := spawn.done
		spawn.mu.Unlock()
		if !done {
			out = append(out, AppletProcess{Handle: handle, Applet: spawn.applet, Command: spawn.command, StartedMillis: spawn.started.UnixMilli()})
		}
	}
	slices.SortFunc(out, func(a, b AppletProcess) int {
		return cmp.Or(cmp.Compare(a.StartedMillis, b.StartedMillis), cmp.Compare(a.Handle, b.Handle))
	})
	return out
}

// KillSpawns stops the processes one frame started, and is what closing that frame
// means. It takes handles rather than an applet id because an applet can have several
// frames open, and closing its panel must not stop what its title bar page runs.
func (s *AppletsService) KillSpawns(handles []string) {
	s.spawns.mu.Lock()
	defer s.spawns.mu.Unlock()
	for _, handle := range handles {
		if spawn, ok := s.spawns.live[handle]; ok {
			spawn.cancel()
			delete(s.spawns.live, handle)
		}
	}
}

// Stop ends every process any applet started.
func (s *AppletsService) Stop() {
	s.spawns.mu.Lock()
	defer s.spawns.mu.Unlock()
	for handle, spawn := range s.spawns.live {
		spawn.cancel()
		delete(s.spawns.live, handle)
	}
}
