package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBuiltInPrefixes(t *testing.T) {
	t.Setenv("HIVE_TEST_SECRET", "from-env")

	path := filepath.Join(t.TempDir(), "token")
	// The trailing newline a heredoc or an editor leaves must not reach the
	// Authorization header.
	require.NoError(t, os.WriteFile(path, []byte("from-file\n"), 0o600))

	env, err := Resolve("env:HIVE_TEST_SECRET")
	require.NoError(t, err)
	assert.Equal(t, "from-env", env)

	file, err := Resolve("file:" + path)
	require.NoError(t, err)
	assert.Equal(t, "from-file", file)
}

func TestResolveReportsAMissingSource(t *testing.T) {
	_, err := Resolve("env:HIVE_TEST_SECRET_THAT_IS_NOT_SET")
	require.Error(t, err)

	_, err = Resolve("file:" + filepath.Join(t.TempDir(), "absent"))
	require.Error(t, err)
}

// A 1Password reference must reach op in the canonical form 1Password's own
// "Copy Secret Reference" produces, not as the remainder after the prefix cut.
func TestOnePasswordRequiresACanonicalReference(t *testing.T) {
	_, err := resolveOnePassword("Private/Grafana/credential")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "op://<vault>/<item>/<field>")
}

func TestOnePasswordReportsAMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := locateOp([]string{t.TempDir()})
	require.ErrorIs(t, err, ErrNoOnePassword)
}

// The prefix search is the whole point: a desktop launch gets a PATH that holds
// no package manager's op.
func TestLocateOpSearchesPrefixesWhenPathMisses(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "op"), []byte("#!/bin/sh\n"), 0o755))

	// A non-executable file in an earlier prefix is skipped, not returned.
	earlier := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(earlier, "op"), []byte("data"), 0o600))

	found, err := locateOp([]string{earlier, dir})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "op"), found)
}

// The rule config depends on: a reference names a source, a literal does not.
func TestHasKnownPrefix(t *testing.T) {
	for _, ref := range []string{
		"env:NAME",
		"file:/etc/hive/token",
		"op://Private/Grafana Cloud/credential",
	} {
		assert.True(t, HasKnownPrefix(ref), ref)
	}
	for _, ref := range []string{
		"",
		"glc_eyJvIjoiMTIzNDU2In0=",
		"vault:kv/data/app",
		"postgres://user:pw@host/db",
	} {
		assert.False(t, HasKnownPrefix(ref), ref)
	}
}

// Registration happens through package-variable initialization, so importing
// this package is what makes the prefix available.
func TestOnePasswordPrefixIsRegistered(t *testing.T) {
	_, err := Resolve("op:not-a-reference")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1Password secret reference")
}
