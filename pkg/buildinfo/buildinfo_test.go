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

// A test binary carries no VCS stamp, so the substitutions run here against a
// given one.
func TestResolveFillsADevBuildFromTheToolchainStamp(t *testing.T) {
	dev := Info{Version: "dev", Commit: "HEAD", Date: "now"}
	for _, tt := range []struct {
		name    string
		in      Info
		vcs     VCS
		stamped bool
		want    Info
	}{
		{"go install module@version", dev, VCS{ModuleVersion: "v1.4.0", Revision: "abc123", Time: "2026-10-02T00:00:00Z"}, true, Info{Version: "v1.4.0", Commit: "abc123", Date: "2026-10-02T00:00:00Z"}},
		{"go build in a checkout", dev, VCS{ModuleVersion: "(devel)", Revision: "abc123", Time: "2026-10-02T00:00:00Z"}, true, Info{Version: "dev", Commit: "abc123", Date: "2026-10-02T00:00:00Z"}},
		{"-buildvcs=false", dev, VCS{ModuleVersion: "(devel)"}, true, dev},
		{"no build information", dev, VCS{ModuleVersion: "v9.9.9", Revision: "zzz"}, false, dev},
		{"a release build keeps its stamp", Info{Version: "v1.2.3", Commit: "def456", Date: "2026-09-01"}, VCS{ModuleVersion: "v9.9.9", Revision: "zzz", Time: "later"}, true, Info{Version: "v1.2.3", Commit: "def456", Date: "2026-09-01"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.in, tt.vcs, tt.stamped); got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
