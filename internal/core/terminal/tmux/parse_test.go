package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePaneLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want paneLine
		ok   bool
	}{
		{
			name: "8 fields with hive tag",
			line: "sess\t1\twname\t/work\t12345\t%2\t6789\tmy-slug",
			want: paneLine{
				sessName: "sess", winIdx: "1", winName: "wname", workDir: "/work",
				activity: 12345, paneID: "%2", panePID: 6789, hiveSession: "my-slug",
			},
			ok: true,
		},
		{
			name: "7 fields (no hive tag)",
			line: "sess\t0\tbash\t/home\t100\t%0\t1234",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 100, paneID: "%0", panePID: 1234,
			},
			ok: true,
		},
		{
			name: "6 fields (no panePID, no tag)",
			line: "sess\t0\tbash\t/home\t100\t%0",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 100, paneID: "%0",
			},
			ok: true,
		},
		{
			name: "5 fields rejected",
			line: "sess\t0\tbash\t/home\t100",
			ok:   false,
		},
		{
			name: "empty rejected",
			line: "",
			ok:   false,
		},
		{
			name: "malformed activity treated as zero",
			line: "sess\t0\tbash\t/home\tnotanumber\t%0\t1234",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 0, paneID: "%0", panePID: 1234,
			},
			ok: true,
		},
		{
			name: "tag with surrounding whitespace trimmed",
			line: "sess\t1\twname\t/work\t12345\t%2\t6789\t  my-slug  ",
			want: paneLine{
				sessName: "sess", winIdx: "1", winName: "wname", workDir: "/work",
				activity: 12345, paneID: "%2", panePID: 6789, hiveSession: "my-slug",
			},
			ok: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePaneLine(tt.line)
			assert.Equal(t, tt.ok, ok)
			if !tt.ok {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSelectPrimary(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		id, pid, all := selectPrimary(nil, "any-slug")
		assert.Empty(t, id)
		assert.Zero(t, pid)
		assert.Empty(t, all)
	})

	t.Run("single untagged pane is primary", func(t *testing.T) {
		panes := []paneLine{{paneID: "%0", panePID: 100}}
		id, pid, all := selectPrimary(panes, "slug")
		assert.Equal(t, "%0", id)
		assert.Equal(t, int64(100), pid)
		assert.Equal(t, []string{"%0"}, all)
	})

	t.Run("single tagged pane is primary", func(t *testing.T) {
		panes := []paneLine{{paneID: "%5", panePID: 200, hiveSession: "slug"}}
		id, pid, all := selectPrimary(panes, "slug")
		assert.Equal(t, "%5", id)
		assert.Equal(t, int64(200), pid)
		assert.Equal(t, []string{"%5"}, all)
	})

	t.Run("tagged pane wins over untagged sibling", func(t *testing.T) {
		panes := []paneLine{
			{paneID: "%0", panePID: 100},                        // untagged user split
			{paneID: "%1", panePID: 101, hiveSession: "myslug"}, // hive pane
		}
		id, _, all := selectPrimary(panes, "myslug")
		assert.Equal(t, "%1", id)
		assert.Equal(t, []string{"%0", "%1"}, all)
	})

	t.Run("matching tag wins over non-matching tag", func(t *testing.T) {
		// Defends against tmux propagating @hive-session to user-split panes:
		// the pane whose tag matches expectedSlug is the original.
		panes := []paneLine{
			{paneID: "%0", panePID: 100, hiveSession: "other-slug"}, // wrong tag
			{paneID: "%1", panePID: 101, hiveSession: "myslug"},     // match
		}
		id, pid, _ := selectPrimary(panes, "myslug")
		assert.Equal(t, "%1", id)
		assert.Equal(t, int64(101), pid)
	})

	t.Run("first tagged wins when no match found", func(t *testing.T) {
		panes := []paneLine{
			{paneID: "%0", panePID: 100, hiveSession: "other"},
			{paneID: "%1", panePID: 101, hiveSession: "another"},
		}
		id, _, _ := selectPrimary(panes, "myslug")
		assert.Equal(t, "%0", id)
	})

	t.Run("empty expectedSlug skips match phase", func(t *testing.T) {
		panes := []paneLine{
			{paneID: "%0", panePID: 100},
			{paneID: "%1", panePID: 101, hiveSession: "tagged"},
		}
		id, _, _ := selectPrimary(panes, "")
		assert.Equal(t, "%1", id)
	})

	t.Run("preserves all pane IDs in input order", func(t *testing.T) {
		panes := []paneLine{
			{paneID: "%2"},
			{paneID: "%0"},
			{paneID: "%5"},
		}
		_, _, all := selectPrimary(panes, "")
		require.Equal(t, []string{"%2", "%0", "%5"}, all)
	})
}
