// Package driveremote copies Hive Drive documents to and from a drive server.
package driveremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/randid"
)

const (
	provider = "drive"

	// EnvServer names the server for headless use, beside the credentials
	// package's HIVE_DRIVE_TOKEN override.
	EnvServer = "HIVE_DRIVE_SERVER"
)

var (
	// ErrNoServer means no flag, environment variable, or stored login names
	// a server.
	ErrNoServer = errors.New("no drive server: run 'hive drive login <server>', or set HIVE_DRIVE_SERVER and HIVE_DRIVE_TOKEN")
	// ErrStoreCredential means the login was valid but the token could not be
	// saved, which is the state of a machine with no keychain.
	ErrStoreCredential = errors.New("could not store the drive token")
	// ErrNotSignedIn means no token is stored or set for the server.
	ErrNotSignedIn = errors.New("not signed in to the drive server")
)

type Action string

const (
	ActionUploaded  Action = "uploaded"
	ActionUnchanged Action = "unchanged"
	ActionCreated   Action = "created"
	// ActionConflict means the download was kept beside a different local
	// file; PullResult.LocalPath names the copy.
	ActionConflict Action = "conflict"
	ActionFailed   Action = "failed"
)

type UploadStatus string

const (
	StatusUploaded UploadStatus = "uploaded"
	StatusChanged  UploadStatus = "changed"
)

type PushResult struct {
	Path   string `json:"path"`
	Action Action `json:"action"`
	SHA256 string `json:"sha256,omitempty"`
	Error  string `json:"error,omitempty"`
}

type PullResult struct {
	Path      string `json:"path"`
	Action    Action `json:"action"`
	SHA256    string `json:"sha256,omitempty"`
	Error     string `json:"error,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
}

type Service struct {
	log     zerolog.Logger
	files   *drivesvc.Service
	uploads drive.UploadStore
	creds   credentials.Store
	getenv  func(string) string
}

func New(log zerolog.Logger, files *drivesvc.Service, uploads drive.UploadStore, creds credentials.Store, getenv func(string) string) *Service {
	return &Service{log: logutils.Component(log, "driveremote"), files: files, uploads: uploads, creds: creds, getenv: getenv}
}

// Login checks the token against the server, then stores it. A token the
// server rejects is never stored.
func (s *Service) Login(ctx context.Context, server, token string) (driveapi.Me, error) {
	host, err := driveapi.Host(server)
	if err != nil {
		return driveapi.Me{}, err
	}
	client, err := driveapi.NewClient(server, func() string { return token })
	if err != nil {
		return driveapi.Me{}, err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return driveapi.Me{}, err
	}
	if err := s.creds.Set(credentials.Ref{Provider: provider, Account: host}, token); err != nil {
		return driveapi.Me{}, fmt.Errorf("%w: %w", ErrStoreCredential, err)
	}
	return me, nil
}

// DefaultServer is flag when set, else HIVE_DRIVE_SERVER, else the one server
// with a stored login.
func (s *Service) DefaultServer(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := s.getenv(EnvServer); env != "" {
		return env, nil
	}
	refs, err := credentials.ListProvider(s.creds, provider)
	if err != nil {
		s.log.Debug().Err(err).Msg("list stored drive logins")
		return "", ErrNoServer
	}
	switch len(refs) {
	case 0:
		return "", ErrNoServer
	case 1:
		return refs[0].Account, nil
	default:
		return "", fmt.Errorf("%d drive servers are signed in: pass --server", len(refs))
	}
}

// Push uploads the markdown and html documents at paths, or the whole space
// when paths is empty. A document whose hash matches its upload record is
// skipped unless force is set.
//
// A failure that belongs to one document (too large, over quota, rejected)
// is that document's result and the push goes on. A failure that would
// repeat for every document (bad credentials, no network) ends the push.
func (s *Service) Push(ctx context.Context, server string, space drive.Space, paths []string, force bool) ([]PushResult, error) {
	client, host, err := s.client(server)
	if err != nil {
		return nil, err
	}
	targets, results, err := s.pushTargets(ctx, space, paths)
	if err != nil {
		return nil, err
	}
	records, err := s.uploads.List(ctx, host, space)
	if err != nil {
		return nil, err
	}

	for _, p := range targets {
		result, err := s.pushOne(ctx, client, host, space, p, records[p], force)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	slices.SortFunc(results, func(a, b PushResult) int { return strings.Compare(a.Path, b.Path) })
	return results, nil
}

func (s *Service) pushOne(ctx context.Context, client *driveapi.Client, host string, space drive.Space, p string, record drive.Upload, force bool) (PushResult, error) {
	_, body, err := s.files.Read(ctx, space, p)
	if err != nil {
		return PushResult{Path: p, Action: ActionFailed, Error: err.Error()}, nil
	}
	sum := hashOf(body)
	if !force && record.SHA256 == sum {
		return PushResult{Path: p, Action: ActionUnchanged, SHA256: sum}, nil
	}

	doc, err := client.Put(ctx, space, p, body)
	if err != nil {
		result := PushResult{Path: p, Action: ActionFailed, SHA256: sum, Error: err.Error()}
		if isPerDocument(err) {
			return result, nil
		}
		return result, err
	}
	err = s.uploads.Record(ctx, drive.Upload{
		ID:         randid.UUIDv7(),
		Server:     host,
		Space:      space,
		Path:       p,
		SHA256:     doc.SHA256,
		UploadedAt: time.Now(),
	})
	if err != nil {
		return PushResult{Path: p, Action: ActionFailed, SHA256: sum, Error: err.Error()}, err
	}
	return PushResult{Path: p, Action: ActionUploaded, SHA256: doc.SHA256}, nil
}

func isPerDocument(err error) bool {
	apiErr, ok := errors.AsType[*driveapi.APIError](err)
	return ok && apiErr.Status != http.StatusUnauthorized && apiErr.Status != http.StatusForbidden
}

// A path that names nothing pushable is returned as a failed result, not an
// error.
func (s *Service) pushTargets(ctx context.Context, space drive.Space, paths []string) ([]string, []PushResult, error) {
	root, err := s.files.Tree(ctx, space)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return pushableUnder(root), nil, nil
	}

	var targets []string
	var failed []PushResult
	for _, raw := range paths {
		p := cleanPath(raw)
		entry, ok := drive.Find(root, p)
		if !ok || p == "" {
			failed = append(failed, PushResult{Path: raw, Action: ActionFailed, Error: "not found in the space"})
			continue
		}
		found := pushableUnder(entry)
		if len(found) == 0 {
			failed = append(failed, PushResult{Path: p, Action: ActionFailed, Error: "only markdown and html documents can be pushed"})
			continue
		}
		targets = append(targets, found...)
	}
	slices.Sort(targets)
	return slices.Compact(targets), failed, nil
}

func pushableUnder(entry drive.Entry) []string {
	var out []string
	var walk func(e drive.Entry)
	walk = func(e drive.Entry) {
		if e.Kind == drive.KindMarkdown || e.Kind == drive.KindHtml {
			out = append(out, e.Path)
		}
		for _, child := range e.Children {
			walk(child)
		}
	}
	walk(entry)
	return out
}

// Pull downloads the documents at paths (a folder pulls everything under
// it), or every document of the space when paths is empty. It never
// overwrites and never deletes: a download that differs from the local
// document is written beside it as <stem>-N<ext>.
func (s *Service) Pull(ctx context.Context, server string, space drive.Space, paths []string) ([]PullResult, error) {
	client, _, err := s.client(server)
	if err != nil {
		return nil, err
	}
	remote, err := client.List(ctx, space)
	if err != nil {
		return nil, err
	}
	wanted, results := selectRemote(remote, paths)
	slices.SortFunc(wanted, func(a, b driveapi.Document) int { return strings.Compare(a.Path, b.Path) })

	root, err := s.files.Tree(ctx, space)
	if err != nil {
		return nil, err
	}
	local := indexTree(root)

	for _, doc := range wanted {
		result, err := s.pullOne(ctx, client, space, local, doc)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	slices.SortFunc(results, func(a, b PullResult) int { return strings.Compare(a.Path, b.Path) })
	return results, nil
}

func selectRemote(remote []driveapi.Document, paths []string) ([]driveapi.Document, []PullResult) {
	if len(paths) == 0 {
		return remote, nil
	}
	var wanted []driveapi.Document
	var failed []PullResult
	seen := map[string]bool{}
	for _, raw := range paths {
		p := cleanPath(raw)
		matched := false
		for _, doc := range remote {
			if doc.Path == p || strings.HasPrefix(doc.Path, p+"/") {
				matched = true
				if !seen[doc.Path] {
					seen[doc.Path] = true
					wanted = append(wanted, doc)
				}
			}
		}
		if !matched {
			failed = append(failed, PullResult{Path: raw, Action: ActionFailed, Error: "not found on the server"})
		}
	}
	return wanted, failed
}

// pullOne compares the listing hash with the local copies first, so an
// unchanged document costs no download. The error is set only when the
// download fails for a reason that repeats for every document.
func (s *Service) pullOne(ctx context.Context, client *driveapi.Client, space drive.Space, local *localIndex, doc driveapi.Document) (PullResult, error) {
	p := doc.Path
	matched, err := s.matchesLocal(ctx, space, local, p, doc.SHA256)
	if err != nil {
		return PullResult{Path: p, Action: ActionFailed, SHA256: doc.SHA256, Error: err.Error()}, nil
	}
	if matched {
		return PullResult{Path: p, Action: ActionUnchanged, SHA256: doc.SHA256}, nil
	}

	_, body, err := client.Get(ctx, space, p)
	if err != nil {
		result := PullResult{Path: p, Action: ActionFailed, Error: err.Error()}
		if isPerDocument(err) {
			return result, nil
		}
		return result, err
	}
	return s.writeDownload(ctx, space, local, p, body), nil
}

func (s *Service) matchesLocal(ctx context.Context, space drive.Space, local *localIndex, p, sha string) (bool, error) {
	for _, candidate := range local.sameDocument(p) {
		_, current, err := s.files.Read(ctx, space, candidate)
		if err != nil {
			return false, err
		}
		if hashOf(current) == sha {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) writeDownload(ctx context.Context, space drive.Space, local *localIndex, p string, body []byte) PullResult {
	sum := hashOf(body)
	fail := func(err error) PullResult {
		return PullResult{Path: p, Action: ActionFailed, SHA256: sum, Error: err.Error()}
	}

	if _, found := local.lookup(p); !found {
		if _, err := s.files.Write(ctx, space, p, drive.Author{}, body); err != nil {
			return fail(err)
		}
		local.add(p)
		return PullResult{Path: p, Action: ActionCreated, SHA256: sum}
	}

	copyPath := local.freeCopy(p)
	if _, err := s.files.Write(ctx, space, copyPath, drive.Author{}, body); err != nil {
		return fail(err)
	}
	local.add(copyPath)
	return PullResult{Path: p, Action: ActionConflict, SHA256: sum, LocalPath: copyPath}
}

func (s *Service) Share(ctx context.Context, server string, space drive.Space, p, expiresIn string) (driveapi.Share, error) {
	client, _, err := s.client(server)
	if err != nil {
		return driveapi.Share{}, err
	}
	return client.Share(ctx, space, cleanPath(p), expiresIn)
}

// Status marks each local document that has an upload record on server as
// uploaded or changed since. It reads the records and the files, never the
// network. A document with no record has no entry.
func (s *Service) Status(ctx context.Context, server string, space drive.Space) (map[string]UploadStatus, error) {
	host, err := driveapi.Host(server)
	if err != nil {
		return nil, err
	}
	records, err := s.uploads.List(ctx, host, space)
	if err != nil {
		return nil, err
	}
	out := make(map[string]UploadStatus, len(records))
	for p, record := range records {
		_, body, err := s.files.Read(ctx, space, p)
		if errors.Is(err, drive.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if hashOf(body) == record.SHA256 {
			out[p] = StatusUploaded
		} else {
			out[p] = StatusChanged
		}
	}
	return out, nil
}

func (s *Service) client(server string) (*driveapi.Client, string, error) {
	host, err := driveapi.Host(server)
	if err != nil {
		return nil, "", err
	}
	token, err := credentials.Resolve(s.creds, credentials.Ref{Provider: provider, Account: host})
	if err != nil {
		return nil, "", fmt.Errorf("read the drive token: %w", err)
	}
	if token == "" {
		return nil, "", fmt.Errorf("%w %s: run 'hive drive login %s', or set %s", ErrNotSignedIn, host, host, credentials.EnvOverrideName(provider))
	}
	client, err := driveapi.NewClient(server, func() string { return token })
	if err != nil {
		return nil, "", err
	}
	return client, host, nil
}

func hashOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func cleanPath(p string) string {
	p = strings.Trim(path.Clean(strings.TrimSpace(p)), "/")
	if p == "." {
		return ""
	}
	return p
}

// localIndex answers path questions case-insensitively, because the drive
// root may sit on a case-insensitive file system where "Plan.md" and
// "plan.md" are one file.
type localIndex struct {
	byFold map[string]string
	dirs   map[string]bool
}

func indexTree(root drive.Entry) *localIndex {
	idx := &localIndex{byFold: map[string]string{}, dirs: map[string]bool{}}
	var walk func(e drive.Entry)
	walk = func(e drive.Entry) {
		for _, child := range e.Children {
			idx.byFold[strings.ToLower(child.Path)] = child.Path
			if child.Kind == drive.KindDir {
				idx.dirs[child.Path] = true
			}
			walk(child)
		}
	}
	walk(root)
	return idx
}

// lookup returns the local path that p names, which differs from p when only
// the case differs.
func (i *localIndex) lookup(p string) (string, bool) {
	existing, ok := i.byFold[strings.ToLower(p)]
	return existing, ok
}

func (i *localIndex) isDir(p string) bool { return i.dirs[p] }

func (i *localIndex) add(p string) { i.byFold[strings.ToLower(p)] = p }

// freeCopy is p with -N before the extension, for the lowest N that no local
// path takes.
func (i *localIndex) freeCopy(p string) string {
	ext := path.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for n := 1; ; n++ {
		candidate := stem + "-" + strconv.Itoa(n) + ext
		if _, taken := i.lookup(candidate); !taken {
			return candidate
		}
	}
}

// sameDocument lists the local files that already stand for the remote path
// p: the file at p (or a case-only variant), and every <stem>-N<ext> copy of
// it. Matching them keeps a repeated pull from piling up copies.
func (i *localIndex) sameDocument(p string) []string {
	var out []string
	if existing, ok := i.lookup(p); ok && !i.isDir(existing) {
		out = append(out, existing)
	}
	ext := path.Ext(p)
	prefix := strings.ToLower(strings.TrimSuffix(p, ext)) + "-"
	suffix := strings.ToLower(ext)
	for fold, actual := range i.byFold {
		rest, ok := strings.CutPrefix(fold, prefix)
		if !ok || i.isDir(actual) {
			continue
		}
		digits, ok := strings.CutSuffix(rest, suffix)
		if !ok || digits == "" || strings.Trim(digits, "0123456789") != "" {
			continue
		}
		out = append(out, actual)
	}
	slices.Sort(out)
	return out
}
