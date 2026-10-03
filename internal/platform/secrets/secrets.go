// Package secrets resolves the credential references config holds in place of
// credentials themselves.
//
// A reference is appkit/secret's prefixed form — "env:NAME", "file:/path", or
// "op://<vault>/<item>/<field>" for 1Password, which this package registers.
// Config therefore names where a secret lives, never the secret, which is what
// keeps a dotfiles-managed settings.yaml free of credentials.
//
// References resolve once, at load. A rotated secret needs a relaunch; that is
// the right trade for a source that may prompt for biometric approval, which
// nothing should do per request.
package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hay-kot/appkit/secret"

	"github.com/colonyops/hive/internal/platform/execenv"
)

// OnePasswordPrefix is the scheme of a 1Password secret reference, the value
// 1Password's own UI offers under "Copy Secret Reference".
const OnePasswordPrefix = "op"

// KnownPrefixes is what a reference may name: appkit/secret's two built-ins
// plus ours. It is the vocabulary a config error quotes back.
var KnownPrefixes = []string{"env", "file", OnePasswordPrefix}

// opTimeout bounds the 1Password lookup. It is generous because the first read
// of a session can raise a Touch ID prompt that waits on a person.
const opTimeout = 30 * time.Second

var ErrNoOnePassword = errors.New("secrets: the 1Password CLI (op) was not found")

// Package-variable initialization rather than init(): gochecknoinits is on, and
// appkit/secret requires registration before the first resolve. Importing this
// package is therefore what makes "op" available.
var _ = register()

func register() bool {
	secret.Register(OnePasswordPrefix, resolveOnePassword)
	return true
}

// Resolve reads the secret a reference names. An unprefixed value resolves to
// itself, so callers that must not accept a literal check [HasKnownPrefix]
// first.
func Resolve(ref string) (string, error) {
	var s secret.Secret
	if err := s.UnmarshalText([]byte(ref)); err != nil {
		return "", err
	}
	return s.Value(), nil
}

// HasKnownPrefix reports whether ref names a source rather than being a literal
// secret pasted into config.
func HasKnownPrefix(ref string) bool {
	prefix, _, ok := strings.Cut(ref, ":")
	return ok && slices.Contains(KnownPrefixes, prefix)
}

// resolveOnePassword receives everything after "op:", so a canonical
// "op://vault/item/field" arrives here as "//vault/item/field".
func resolveOnePassword(rest string) (string, error) {
	if !strings.HasPrefix(rest, "//") {
		return "", fmt.Errorf("secrets: %q is not a 1Password secret reference; use op://<vault>/<item>/<field>", OnePasswordPrefix+":"+rest)
	}
	ref := OnePasswordPrefix + ":" + rest

	bin, err := locateOp(execenv.SearchDirs())
	if err != nil {
		return "", err
	}

	//nolint:forbidigo // a secret.Resolver takes no context; opTimeout is the bound
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "read", "--no-newline", ref)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("secrets: read %s: %w: %s", ref, err, detail)
		}
		return "", fmt.Errorf("secrets: read %s: %w", ref, err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// locateOp mirrors tmuxbin.Locate: a desktop launch gets
// /usr/bin:/bin:/usr/sbin:/sbin, which holds no package manager's op.
func locateOp(dirs []string) (string, error) {
	if path, err := exec.LookPath(OnePasswordPrefix); err == nil {
		return path, nil
	}
	for _, dir := range dirs {
		candidate := filepath.Join(dir, OnePasswordPrefix)
		if executable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w on PATH or in %s", ErrNoOnePassword, strings.Join(dirs, ", "))
}

// executable follows symlinks, because a Homebrew or Nix bin entry is a link
// into a versioned prefix.
func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}
