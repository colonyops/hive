package buildinfo

import "testing"

func TestResolveKeepsStampedValues(t *testing.T) {
	got := Resolve("v1.2.3", "abc123", "2026-10-02")
	want := Info{Version: "v1.2.3", Commit: "abc123", Date: "2026-10-02"}
	if got != want {
		t.Errorf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestResolveDevBuildNeverBlanksAField(t *testing.T) {
	got := Resolve("dev", "HEAD", "now")
	if got.Version == "" || got.Commit == "" || got.Date == "" {
		t.Errorf("Resolve(dev) = %+v, want every field set", got)
	}
}
