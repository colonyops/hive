package hivecli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hivecli"
)

type fixture struct {
	home    string
	exe     string
	path    string
	manager *hivecli.Manager
}

type fixtureOptions struct {
	version  string
	exe      string
	userPath func(home string) string
	probe    func(path string) (string, error)
}

func newFixture(t *testing.T, o fixtureOptions) *fixture {
	t.Helper()
	home := t.TempDir()
	exe := o.exe
	if exe == "" {
		exe = filepath.Join(home, "Applications", "Hive.app", "Contents", "MacOS", "hive-desktop")
		require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0o755))
		require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755))
	}
	version := o.version
	if version == "" {
		version = "1.2.0"
	}
	f := &fixture{home: home, exe: exe, path: filepath.Join(home, ".local", "bin", "hive")}
	f.manager = hivecli.New(hivecli.Options{
		Home:       home,
		Version:    version,
		Executable: func() (string, error) { return exe, nil },
		UserPath: func(context.Context) string {
			if o.userPath != nil {
				return o.userPath(home)
			}
			return filepath.Join(home, ".local", "bin")
		},
		Probe: func(_ context.Context, path string, _ []string) (string, error) {
			if o.probe != nil {
				return o.probe(path)
			}
			return "hive version 1.2.0 (abc) now\n", nil
		},
	})
	return f
}

func (f *fixture) link(t *testing.T, target string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(f.path), 0o755))
	require.NoError(t, os.Symlink(target, f.path))
}

func (f *fixture) foreignFile(t *testing.T) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(f.path), 0o755))
	require.NoError(t, os.WriteFile(f.path, []byte("#!/bin/sh\n"), 0o755))
}

func (f *fixture) target(t *testing.T) string {
	t.Helper()
	target, err := os.Readlink(f.path)
	require.NoError(t, err)
	return target
}

func ptr(b bool) *bool { return &b }

func TestReconcileTable(t *testing.T) {
	ctx := t.Context()

	t.Run("missing and enabled creates the link and its directory", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		st := f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, f.exe, f.target(t))
		assert.Equal(t, hivecli.OwnerApp, st.Owner)
		assert.Equal(t, hivecli.SkipNone, st.Skip)
	})

	t.Run("missing and disabled does nothing", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.manager.Reconcile(ctx, ptr(false))
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Lstat(filepath.Dir(f.path))
		assert.ErrorIs(t, err, os.ErrNotExist, "disabling must not create the directory")
	})

	t.Run("never asked writes nothing", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.manager.Reconcile(ctx, nil)
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("app-owned to this executable and enabled is left alone", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.link(t, f.exe)
		before, err := os.Lstat(f.path)
		require.NoError(t, err)
		f.manager.Reconcile(ctx, ptr(true))
		after, err := os.Lstat(f.path)
		require.NoError(t, err)
		assert.True(t, os.SameFile(before, after))
	})

	t.Run("app-owned to this executable and disabled is removed", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.link(t, f.exe)
		st := f.manager.Reconcile(ctx, ptr(false))
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Equal(t, hivecli.OwnerNone, st.Owner)
	})

	t.Run("app-owned elsewhere and enabled is re-pointed", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.link(t, "/Applications/Hive.app/Contents/MacOS/hive-desktop")
		f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, f.exe, f.target(t))
		entries, err := os.ReadDir(filepath.Dir(f.path))
		require.NoError(t, err)
		assert.Len(t, entries, 1, "the staging link is gone")
	})

	t.Run("app-owned elsewhere and disabled is removed", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.link(t, "/opt/hive/hive-desktop")
		f.manager.Reconcile(ctx, ptr(false))
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	for name, setup := range map[string]func(*testing.T, *fixture){
		"file":           func(t *testing.T, f *fixture) { f.foreignFile(t) },
		"link elsewhere": func(t *testing.T, f *fixture) { f.link(t, "/opt/homebrew/bin/hive") },
	} {
		for _, enabled := range []bool{true, false} {
			t.Run("foreign "+name+" is never written or removed", func(t *testing.T) {
				f := newFixture(t, fixtureOptions{})
				setup(t, f)
				before, err := os.Lstat(f.path)
				require.NoError(t, err)
				st := f.manager.Reconcile(ctx, ptr(enabled))
				after, err := os.Lstat(f.path)
				require.NoError(t, err)
				assert.True(t, os.SameFile(before, after))
				assert.Equal(t, hivecli.OwnerForeign, st.Owner)
			})
		}
	}
}

func TestReconcileSkips(t *testing.T) {
	ctx := t.Context()

	t.Run("dev build", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{version: "dev"})
		st := f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, hivecli.SkipDev, st.Skip)
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("translocated path", func(t *testing.T) {
		exe := "/private/var/folders/x/AppTranslocation/ABC/d/Hive.app/Contents/MacOS/hive-desktop"
		f := newFixture(t, fixtureOptions{exe: exe})
		st := f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, hivecli.SkipTranslocated, st.Skip)
		_, err := os.Lstat(f.path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("an executable the ownership rule would not recognize", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{exe: "/usr/local/bin/other-name"})
		st := f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, hivecli.SkipUnrecognizable, st.Skip)
	})

	t.Run("unwritable install directory", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		require.NoError(t, os.MkdirAll(filepath.Join(f.home, ".local"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(f.home, ".local", "bin"), nil, 0o644), "a file where the directory belongs")
		st := f.manager.Reconcile(ctx, ptr(true))
		assert.Equal(t, hivecli.SkipUnwritable, st.Skip)
		assert.NotEmpty(t, st.Detail)

		later := f.manager.Status(ctx)
		assert.Equal(t, hivecli.SkipUnwritable, later.Skip, "Status keeps the last reconcile's failure")
	})
}

func TestOwnershipDetection(t *testing.T) {
	ctx := t.Context()
	for name, tc := range map[string]struct {
		target string
		owner  hivecli.Owner
	}{
		"bundle executable":    {"/Applications/Hive.app/Contents/MacOS/hive-desktop", hivecli.OwnerApp},
		"linux executable":     {"/home/u/.local/share/hive/hive-desktop", hivecli.OwnerApp},
		"dangling app link":    {"/gone/Hive.app/Contents/MacOS/hive-desktop", hivecli.OwnerApp},
		"homebrew":             {"/opt/homebrew/Caskroom/hive/1.0/hive", hivecli.OwnerForeign},
		"go install":           {"/home/u/go/bin/hive", hivecli.OwnerForeign},
		"similar name":         {"/opt/my-hive-desktop", hivecli.OwnerForeign},
		"desktop as directory": {"/opt/hive-desktop/hive", hivecli.OwnerForeign},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, fixtureOptions{})
			f.link(t, tc.target)
			st := f.manager.Status(ctx)
			assert.Equal(t, tc.owner, st.Owner)
			assert.Equal(t, tc.target, st.Target)
		})
	}
}

func TestStatusReportsWhatTheLoginShellResolves(t *testing.T) {
	ctx := t.Context()

	t.Run("installed, on PATH, same version", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{})
		f.manager.Reconcile(ctx, ptr(true))
		st := f.manager.Status(ctx)
		assert.True(t, st.OnPath)
		assert.Equal(t, f.path, st.Resolved)
		assert.False(t, st.Shadowed)
		assert.Equal(t, "1.2.0", st.CommandVersion)
		assert.False(t, st.Differs)
	})

	t.Run("installed but the directory is not on PATH", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{userPath: func(string) string { return "/usr/bin" }})
		f.manager.Reconcile(ctx, ptr(true))
		st := f.manager.Status(ctx)
		assert.False(t, st.OnPath)
		assert.Empty(t, st.Resolved)
		assert.False(t, st.Differs, "nothing resolved, nothing to compare")
	})

	t.Run("a homebrew copy earlier on PATH shadows the link", func(t *testing.T) {
		brew := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(brew, "hive"), []byte("#!/bin/sh\n"), 0o755))
		f := newFixture(t, fixtureOptions{
			userPath: func(home string) string { return brew + ":" + filepath.Join(home, ".local", "bin") },
			probe:    func(string) (string, error) { return "hive version 1.1.0 (abc) now\n", nil },
		})
		f.manager.Reconcile(ctx, ptr(true))
		st := f.manager.Status(ctx)
		assert.True(t, st.OnPath)
		assert.True(t, st.Shadowed)
		assert.Equal(t, filepath.Join(brew, "hive"), st.Resolved)
		assert.Equal(t, "1.1.0", st.CommandVersion)
		assert.True(t, st.Differs)
	})

	t.Run("an unparseable version differs", func(t *testing.T) {
		f := newFixture(t, fixtureOptions{probe: func(string) (string, error) { return "", errors.New("timeout") }})
		f.manager.Reconcile(ctx, ptr(true))
		st := f.manager.Status(ctx)
		assert.Empty(t, st.CommandVersion)
		assert.True(t, st.Differs)
	})
}

func TestParseVersion(t *testing.T) {
	got, ok := hivecli.ParseVersion("hive version v0.5.0 (abc) now\n")
	assert.True(t, ok)
	assert.Equal(t, "v0.5.0", got)

	for _, in := range []string{"garbage", "hive version ", ""} {
		_, ok := hivecli.ParseVersion(in)
		assert.False(t, ok, in)
	}
}
