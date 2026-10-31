package app

import (
	"bytes"
	"context"
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
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/pathutil"
)

// This file is what window.hive reaches. An html applet is a page the user asked
// for on their own machine, so these calls are deliberately unrestricted: any
// command, any path (ADR applets-are-declared-in-yaml-and-rendered-natively).

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
}

func (s *AppletsService) page(id string) (applets.Applet, error) {
	applet, err := s.applet(id)
	if err != nil {
		return applets.Applet{}, err
	}
	if applet.View != applets.ViewHTML {
		return applets.Applet{}, Errorf(KindInvalid, "applet %q is not an html applet", id)
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
	if !filepath.IsLocal(name) {
		return AppletAsset{}, Errorf(KindNotFound, "no file %q in applet %q", name, id)
	}
	body, err := os.ReadFile(filepath.Join(applet.Dir, filepath.FromSlash(name)))
	if err != nil {
		return AppletAsset{}, Wrap(err, KindNotFound, "reading %q of applet %q", name, id)
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	if strings.HasPrefix(contentType, "text/html") {
		body = injectBridge(body, readAppletStorage(applet.Dir))
	}
	return AppletAsset{Name: name, ContentType: contentType, Body: body}, nil
}

var htmlHead = regexp.MustCompile(`(?i)<head[^>]*>`)

func injectBridge(page []byte, storage []byte) []byte {
	// The JSON sits inside a script element, so a stored "</script>" must not end it.
	var safe bytes.Buffer
	json.HTMLEscape(&safe, storage)
	script := []byte("<script>window.__hiveStorage=" + safe.String() + ";" + applets.BridgeScript() + "</script>")
	if at := htmlHead.FindIndex(page); at != nil {
		return append(append(append([]byte{}, page[:at[1]]...), script...), page[at[1]:]...)
	}
	return append(script, page...)
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
	return s.WriteFile(id, appletStorageFile, encoded)
}

// command builds the process. The returned context outlives ctx, because a spawned
// process outlives the call that started it; its cancel is what ends the process.
func (s *AppletsService) command(ctx context.Context, id string, req AppletExec) (*exec.Cmd, context.Context, context.CancelFunc, error) {
	applet, err := s.page(id)
	if err != nil {
		return nil, nil, nil, err
	}
	dir, err := s.dir(ctx, applet, req.SessionSlug)
	if err != nil {
		return nil, nil, nil, err
	}
	if req.Cwd != "" {
		cwd, err := pathutil.ExpandHomeE(req.Cwd)
		if err != nil {
			return nil, nil, nil, Wrap(err, KindInternal, "expanding %q", req.Cwd)
		}
		if filepath.IsAbs(cwd) {
			dir = cwd
		} else {
			dir = filepath.Join(dir, cwd)
		}
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	proc := exec.CommandContext(runCtx, "sh", "-c", req.Command)
	proc.Dir = dir
	proc.Env = append(s.environ(ctx), appletEnv(applet)...)
	for _, key := range slices.Sorted(maps.Keys(req.Env)) {
		proc.Env = append(proc.Env, key+"="+req.Env[key])
	}
	return proc, runCtx, cancel, nil
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
	proc, _, cancel, err := s.command(ctx, id, req)
	if err != nil {
		return AppletExecResult{}, err
	}
	defer cancel()
	timeout := appletActionTimeout
	if req.TimeoutMillis > 0 {
		timeout = time.Duration(req.TimeoutMillis) * time.Millisecond
	}
	runCtx, stop := context.WithTimeout(ctx, timeout)
	defer stop()

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
}

type appletSpawn struct {
	applet string
	cancel context.CancelFunc
	stdin  io.WriteCloser

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
	proc, runCtx, cancel, err := s.command(ctx, id, req)
	if err != nil {
		return "", err
	}
	spawn := &appletSpawn{applet: id, cancel: cancel, wake: make(chan struct{}, 1)}
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

	go func() {
		code, err := exitResult(dispatch.RunShellProcess(runCtx, proc))
		spawn.mu.Lock()
		if err != nil {
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
	out := AppletOutput{Stdout: p.stdout.String(), Stderr: p.stderr.String(), Done: p.done, Code: p.code}
	p.stdout.Reset()
	p.stderr.Reset()
	return out, out.Done || out.Stdout != "" || out.Stderr != ""
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

// KillSpawns stops every process an applet started, and is what closing it means.
func (s *AppletsService) KillSpawns(id string) {
	s.spawns.mu.Lock()
	defer s.spawns.mu.Unlock()
	for handle, spawn := range s.spawns.live {
		if spawn.applet == id {
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
