package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBirthTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	before := time.Now().Add(-time.Minute)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := BirthTime(fi)
	if runtime.GOOS != "darwin" {
		if ok {
			t.Fatalf("BirthTime on %s = %v, true; want false", runtime.GOOS, got)
		}
		return
	}
	if !ok {
		t.Fatal("BirthTime on darwin: want a time")
	}
	if got.Before(before) || got.After(fi.ModTime().Add(time.Second)) {
		t.Fatalf("BirthTime = %v; want between %v and the mod time %v", got, before, fi.ModTime())
	}
}
