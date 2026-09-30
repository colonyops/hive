package configmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// migratedFileHeader is prepended to a migrated document before it is written
// back to path. The body is a fresh yaml.Marshal of a map[string]any, so any
// comments in the original file are already gone by the time MigrateFile
// writes it back; this header makes that loss visible on disk instead of
// silent.
const migratedFileHeader = "# rewritten by a config migration; original comments were not retained\n"

// MigrateFile reads path, applies set, and — only when the document actually
// changed (version < Current) — writes a timestamped backup of the pre-migration
// bytes under backupDir, then atomically rewrites path with the migrated bytes.
// It returns the bytes the caller should decode (migrated when changed, the
// original otherwise), whether a write happened, and any error.
//
//   - A missing file is (nil, false, nil): the loaders already treat absence as
//     "use defaults / empty set", so nothing is created here.
//   - An already-current file is a pure no-op: no backup, no write.
//   - An Apply error (including ErrVersionTooNew, a corrupt-file decode error, or
//     a migration step error) writes nothing and is returned.
//   - If the backup write fails (disk full, permission), MigrateFile does NOT
//     rewrite the source — the original stays in place, unmigrated — and returns
//     the error. Backup-before-rewrite means a step error also leaves no
//     half-written artifact and no backup.
//
// backupDir is caller-supplied and MUST be outside any watched flows/actions
// directory (callers pass <StateDir>/migration-backups) — see ADR yaml-config-migration for why
// the startup pass is the sole writer.
func MigrateFile(set Set, path, backupDir string, log *zerolog.Logger) (data []byte, changed bool, err error) {
	l := safeLog(log)

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s config %s: %w", set.Name, path, err)
	}

	migrated, changed, err := set.Apply(raw)
	if err != nil {
		l.Error().Err(err).Str("set", set.Name).Str("file", path).Msg("config migration failed")
		return nil, false, err
	}
	if !changed {
		return migrated, false, nil
	}

	from := preMigrationVersion(set, raw)
	out := append([]byte(migratedFileHeader), migrated...)

	backupPath := filepath.Join(backupDir, backupName(set, path, from, time.Now()))
	if err := writeAtomic(backupPath, raw); err != nil {
		wrapped := fmt.Errorf("backup %s config before migration: %w", set.Name, err)
		l.Error().Err(wrapped).Str("file", path).Str("backupDir", backupDir).
			Msg("config migration backup failed; source left untouched")
		return nil, false, wrapped
	}

	if err := writeAtomic(path, out); err != nil {
		wrapped := fmt.Errorf("rewrite migrated %s config: %w", set.Name, err)
		l.Error().Err(wrapped).Str("file", path).Msg("config migration rewrite failed")
		return nil, false, wrapped
	}

	l.Info().
		Str("set", set.Name).
		Str("file", path).
		Int("from", from).
		Int("to", set.Current).
		Str("backup", backupPath).
		Msg("migrated config file")

	return out, true, nil
}

// safeLog returns *log, or a no-op logger when log is nil, so a caller that
// forgets to pass one cannot panic the migration path.
func safeLog(log *zerolog.Logger) zerolog.Logger {
	if log == nil {
		return zerolog.Nop()
	}
	return *log
}

// preMigrationVersion recovers the version raw carried before Apply migrated
// it. It is only ever called after Apply has already reported changed=true for
// these same raw bytes, so the lax decode here cannot fail in practice; a
// failure defensively falls back to set.Baseline rather than erroring a
// migration that has already succeeded.
func preMigrationVersion(set Set, raw []byte) int {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return set.Baseline
	}
	if doc == nil {
		doc = map[string]any{}
	}
	v, err := set.version(doc)
	if err != nil {
		return set.Baseline
	}
	return v
}

// backupName builds "<set.Name>-<stem>.v<fromVersion>.<UTC timestamp>.bak",
// e.g. "flow-triage.v1.20260728T150405Z.yaml.bak", so a backup is
// self-describing (set, source file, pre-migration version, when) and never
// collides across runs. now is a parameter rather than time.Now() so callers
// can test it deterministically.
func backupName(set Set, srcPath string, fromVersion int, now time.Time) string {
	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	ts := now.UTC().Format("20060102T150405Z")
	return fmt.Sprintf("%s-%s.v%d.%s%s.bak", set.Name, stem, fromVersion, ts, ext)
}

// writeAtomic writes data to path via temp-file-in-same-dir + fsync + rename,
// 0o600 — the same shape as settings.saveSettingsAt and flow.writeFileAtomic,
// duplicated here because configmigrate must not import either package.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s dir: %w", filepath.Base(path), err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", filepath.Base(path), err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file for %s: %w", filepath.Base(path), err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file for %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
