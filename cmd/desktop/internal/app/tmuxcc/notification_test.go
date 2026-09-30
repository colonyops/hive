package tmuxcc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello", "hello"},
		{"octal escapes", `hi\015\012`, "hi\r\n"},
		{"escaped backslash", `a\134b`, `a\b`},
		{"escape sequence", `\033[1mbold\033[0m`, "\x1b[1mbold\x1b[0m"},
		{"high bytes pass through", "caf\xc3\xa9", "café"},
		{"stray control bytes dropped", "a\x01b", "ab"},
		{"truncated escape", `a\01`, "a?"},
		{"non-octal escape", `a\9zz`, "a?9zz"},
		{"trailing backslash", `a\`, "a?"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, string(decodeOutput([]byte(tc.in))))
		})
	}
}

func TestParseNotification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line string
		want Notification
	}{
		{`%output %12 done\015\012`, OutputNotification{Pane: "%12", Data: []byte("done\r\n")}},
		{"%output %12 ", OutputNotification{Pane: "%12", Data: []byte{}}},
		{"%window-add @3", WindowAddNotification{Window: "@3"}},
		{"%window-close @3", WindowCloseNotification{Window: "@3"}},
		// What tmux actually sends for kill-window: the notification is deferred
		// past the point where the window is still linked into the session.
		{"%unlinked-window-close @3", WindowCloseNotification{Window: "@3"}},
		{"%window-renamed @3 my window", WindowRenamedNotification{Window: "@3", Name: "my window"}},
		{"%window-pane-changed @3 %9", WindowPaneChanged{Window: "@3", Pane: "%9"}},
		{"%session-changed $1 hive-demo", SessionChanged{Session: "$1", Name: "hive-demo"}},
		{"%session-window-changed $1 @2", SessionWindowChanged{Session: "$1", Window: "@2"}},
		{"%layout-change @2 b25d,80x24,0,0,1 b25d,80x24,0,0,1 *", LayoutChanged{Window: "@2", Layout: singlePaneLayout("%1", 80, 24)}},
		// The flags say whether the active pane is zoomed; the first layout is
		// still the unzoomed tree, which is what the renderer lays out from.
		{"%layout-change @2 f9e1,213x55,0,0{106x55,0,0,3,106x55,107,0,4} aafe,213x55,0,0,4 *Z", LayoutChanged{
			Window: "@2",
			Zoomed: true,
			Layout: Layout{
				Split: SplitLeftRight, Width: 213, Height: 55,
				Cells: []Layout{
					{Pane: "%3", Width: 106, Height: 55},
					{Pane: "%4", Width: 106, Height: 55, X: 107},
				},
			},
		}},
		{"%layout-change @2", LayoutChanged{Window: "@2"}},
		{"%layout-change @2 garbage", LayoutChanged{Window: "@2"}},
		{"%pause %4", PauseNotification{Pane: "%4"}},
		{"%continue %4", ContinueNotification{Pane: "%4"}},
		{"%exit", ExitNotification{}},
		{"%exit server exited unexpectedly", ExitNotification{Reason: "server exited unexpectedly"}},
	}

	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()
			got, err := parseNotification([]byte(tc.line))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// %extended-output is only emitted under pause mode, which v1 never enables —
// it is parsed so an externally-enabled pause mode cannot break framing.
func TestParseExtendedOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		line string
		want string
	}{
		{"age then payload", `%extended-output %1 4212 : done\015\012`, "done\r\n"},
		{"unknown params skipped", `%extended-output %1 4212 7 x : done`, "done"},
		{"separator inside payload", "%extended-output %1 4212 : a : b", "a : b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseNotification([]byte(tc.line))
			require.NoError(t, err)
			require.Equal(t, OutputNotification{Pane: "%1", Data: []byte(tc.want)}, got)
		})
	}
}

func TestParseNotificationRejects(t *testing.T) {
	t.Parallel()

	lines := []string{
		"%output",
		"%output %notanumber data",
		"%window-add",
		"%window-add @",
		"%window-renamed @1",
		"%session-window-changed $1 notawindow",
		"%pause 4",
		"%extended-output %1 notanumber : data",
		"%extended-output %1 12",
		"%brand-new-notification arg",
		"random garbage",
		"",
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			got, err := parseNotification([]byte(line))
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

func TestValidWindowID(t *testing.T) {
	t.Parallel()

	require.True(t, validWindowID("@1"))
	require.True(t, validWindowID("@275"))
	require.False(t, validWindowID("@"))
	require.False(t, validWindowID("1"))
	require.False(t, validWindowID("@1; kill-server"))
	require.False(t, validWindowID(""))
}
