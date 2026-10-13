package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/pkg/atomicfile"
)

// ImportLegacyCanvases moves the canvases in from, a canvases directory
// outside the drive, to <space>/canvases/<name>.canvas.json, rewriting each
// file's space and path on the way. A .json that is not a canvas stays; a
// target that exists is skipped with a warning; an emptied from is removed.
// A from that does not exist is 0, nil. The desktop runs this at start for
// the directories only it knows, so it is idempotent.
func (s *Service) ImportLegacyCanvases(_ context.Context, from string, space drive.Space) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(dir, drive.CanvasesDir)
	if filepath.Clean(from) == target {
		return 0, nil
	}
	entries, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("drive: import canvases from %s: %w", from, err)
	}

	var moved []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		src := filepath.Join(from, name)
		data, err := os.ReadFile(src)
		if err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas not readable, left in place")
			continue
		}
		if !isCanvasJSON(data) {
			continue
		}
		var c drive.Canvas
		if err := json.Unmarshal(data, &c); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas does not parse, left in place")
			continue
		}
		path := drive.CanvasesDir + "/" + canvasStem(name) + drive.CanvasExt
		dst := filepath.Join(dir, filepath.FromSlash(path))
		if _, err := os.Lstat(dst); err == nil {
			s.logger.Warn().Str("file", src).Str("target", dst).Msg("legacy canvas target exists, left in place")
			continue
		}
		c.Space = space.Key
		c.Path = path
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return len(moved), fmt.Errorf("drive: encode %s: %w", src, err)
		}
		if err := os.MkdirAll(target, dirPerm); err != nil {
			return len(moved), fmt.Errorf("drive: create %s: %w", target, err)
		}
		if err := atomicfile.Write(dst, out, filePerm); err != nil {
			return len(moved), fmt.Errorf("drive: write %s: %w", dst, err)
		}
		if err := os.Remove(src); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas copied but not removed")
		}
		moved = append(moved, path)
	}

	if rest, err := os.ReadDir(from); err == nil && len(rest) == 0 {
		if err := os.Remove(from); err != nil {
			s.logger.Warn().Err(err).Str("dir", from).Msg("emptied legacy canvases directory not removed")
		}
	}
	if len(moved) > 0 {
		delete(s.spaces, space.Key)
		s.publish(space, drive.Author{}, moved...)
	}
	return len(moved), nil
}

// renameLegacyCanvases gives every <name>.json in dir, a space's canvases
// folder, that holds a canvas its .canvas.json name. snapshot runs it once
// per space, before the first scan; a legacy file that appears later stays a
// plain file. A target that exists is left alone with a warning.
func (s *Service) renameLegacyCanvases(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.logger.Warn().Err(err).Str("dir", dir).Msg("canvases folder not listed for the legacy rename")
		}
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, drive.CanvasExt) {
			continue
		}
		src := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil || !isCanvasJSON(data) {
			continue
		}
		dst := filepath.Join(dir, canvasStem(name)+drive.CanvasExt)
		if _, err := os.Lstat(dst); err == nil {
			s.logger.Warn().Str("file", src).Str("target", dst).Msg("legacy canvas target exists, left in place")
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas not renamed")
		}
	}
}

// isCanvasJSON reports whether data is a JSON object with a blocks key.
// Canvas.Blocks has no omitempty, so every canvas has a blocks key, and no
// other file in a canvases directory does.
func isCanvasJSON(data []byte) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return false
	}
	_, ok := object["blocks"]
	return ok
}

func canvasStem(filename string) string {
	stem := strings.TrimSuffix(filename, ".json")
	return strings.TrimSuffix(stem, ".canvas")
}
