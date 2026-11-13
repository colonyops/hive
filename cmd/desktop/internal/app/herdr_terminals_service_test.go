package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/platform/herdrcore/hctest"
)

type fixedDirectory map[string]string

func (d fixedDirectory) SessionDirectory(_ context.Context, slug string) (string, error) {
	if dir, ok := d[slug]; ok {
		return dir, nil
	}
	return "", Errorf(KindNotFound, "no session %q", slug)
}

func TestHerdrTerminalsStartInTheCheckoutAndKill(t *testing.T) {
	socket := hctest.Start(t)
	dir := t.TempDir()
	s := newHerdrTerminalsService(HerdrTerminalsDeps{
		Environ:   func(context.Context) []string { return []string{"HC_SOCKET=" + socket} },
		LookPath:  func(context.Context, string) (string, error) { return "", errors.New("unused") },
		Directory: fixedDirectory{"web": dir},
		Logger:    zerolog.Nop(),
	})
	t.Cleanup(func() { _ = s.Close() })
	ctx := t.Context()

	term, err := s.Start(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if term.Labels[herdrSessionLabel] != "web" || !term.Keep {
		t.Fatalf("record %+v", term)
	}
	got, err := s.Get(ctx, term.Address)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cwd != dir {
		t.Errorf("cwd %q, want %q", got.Cwd, dir)
	}

	if err := s.Kill(ctx, term.Address); err != nil {
		t.Fatal(err)
	}
	if err := s.Kill(ctx, term.Address); err != nil {
		t.Errorf("killing an ended terminal again: %v", err)
	}
	if _, err := s.Start(ctx, "missing"); KindOf(err) != KindNotFound {
		t.Errorf("unknown session: %v", err)
	}
	if _, err := s.Start(ctx, "box:web"); KindOf(err) != KindUnavailable {
		t.Errorf("remote session: %v", err)
	}
}
