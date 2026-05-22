package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePaneLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want paneLine
		ok   bool
	}{
		{
			name: "9 fields with pane title and hive tag",
			line: "sess|||1|||wname|||/work|||12345|||%2|||6789|||ptitle|||my-slug",
			want: paneLine{
				sessName: "sess", winIdx: "1", winName: "wname", workDir: "/work",
				activity: 12345, paneID: "%2", panePID: 6789, paneTitle: "ptitle", hiveSession: "my-slug",
			},
			ok: true,
		},
		{
			name: "8 fields with pane title and no hive tag",
			line: "sess|||0|||bash|||/home|||100|||%0|||1234|||ptitle",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 100, paneID: "%0", panePID: 1234, paneTitle: "ptitle",
			},
			ok: true,
		},
		{
			name: "6 fields (no panePID, no tag)",
			line: "sess|||0|||bash|||/home|||100|||%0",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 100, paneID: "%0",
			},
			ok: true,
		},
		{
			name: "5 fields rejected",
			line: "sess|||0|||bash|||/home|||100",
			ok:   false,
		},
		{
			name: "empty rejected",
			line: "",
			ok:   false,
		},
		{
			name: "malformed activity treated as zero",
			line: "sess|||0|||bash|||/home|||notanumber|||%0|||1234",
			want: paneLine{
				sessName: "sess", winIdx: "0", winName: "bash", workDir: "/home",
				activity: 0, paneID: "%0", panePID: 1234,
			},
			ok: true,
		},
		{
			name: "tag with surrounding whitespace trimmed",
			line: "sess|||1|||wname|||/work|||12345|||%2|||6789|||ptitle|||  my-slug  ",
			want: paneLine{
				sessName: "sess", winIdx: "1", winName: "wname", workDir: "/work",
				activity: 12345, paneID: "%2", panePID: 6789, paneTitle: "ptitle", hiveSession: "my-slug",
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

func TestParsePaneList(t *testing.T) {
	got := parsePaneList("sess|||0|||claude|||/work|||100|||%1|||123|||ptitle|||my-slug\n")
	assert.Len(t, got, 1)
	assert.Equal(t, "sess", got[0].SessionName)
	assert.Equal(t, "%1", got[0].PaneID)
	assert.Equal(t, int64(123), got[0].PanePID)
	assert.Equal(t, "ptitle", got[0].PaneTitle)
	assert.Equal(t, "my-slug", got[0].HiveSession)
}
