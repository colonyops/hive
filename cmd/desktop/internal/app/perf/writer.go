package perf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// sink is an append-only JSONL file that caps its own size. Rotation keeps one
// previous generation (path + ".1") so a rotation mid-session does not lose the
// run being debugged; worst-case disk use is 2 × max.
//
// Not safe for concurrent use — Recorder owns the only reference and holds its
// own lock across every call.
type sink struct {
	f    *os.File
	path string
	max  int64
	size int64
}

func openSink(path string, max int64) (*sink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create perf dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open perf file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat perf file: %w", err)
	}
	return &sink{f: f, path: path, max: max, size: info.Size()}, nil
}

// write encodes v as one JSON line, rotating first when the line would carry
// the file past its cap. The line is marshalled up front so a rotation
// decision is made against its real size and a failed encode cannot leave a
// half-written record behind.
func (s *sink) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode perf sample: %w", err)
	}
	line = append(bytes.TrimRight(line, "\n"), '\n')

	if s.size > 0 && s.size+int64(len(line)) > s.max {
		if err := s.rotate(); err != nil {
			return err
		}
	}

	n, err := s.f.Write(line)
	s.size += int64(n)
	if err != nil {
		return fmt.Errorf("write perf sample: %w", err)
	}
	return nil
}

func (s *sink) rotate() error {
	if err := s.f.Close(); err != nil {
		return fmt.Errorf("close perf file for rotation: %w", err)
	}
	if err := os.Rename(s.path, s.path+".1"); err != nil {
		return fmt.Errorf("rotate perf file: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("reopen perf file after rotation: %w", err)
	}
	s.f = f
	s.size = 0
	return nil
}

func (s *sink) close() error { return s.f.Close() }
