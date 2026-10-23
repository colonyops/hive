package tmuxcc

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageDetectorFindsACommandSplitAcrossOutput(t *testing.T) {
	t.Parallel()

	command := []byte("ab\x1b_Ga=T;xyz")
	for split := range len(command) + 1 {
		d := newImageDetector()
		first := d.first("%1", command[:split])
		second := d.first("%1", command[split:])
		require.NotEqual(t, first, second, "split at %d reports exactly once", split)
	}
}

func TestImageDetectorKeepsPanesApartAndReportsOnce(t *testing.T) {
	t.Parallel()

	d := newImageDetector()
	require.False(t, d.first("%1", []byte("\x1b_")))
	require.False(t, d.first("%2", []byte("G")), "another pane's bytes do not complete it")
	require.True(t, d.first("%1", []byte("G")))
	require.False(t, d.first("%1", []byte("\x1b_G")), "a pane is reported once")
	require.False(t, d.first("%3", []byte("\x1b_Ttitle\x1b\\")), "other APCs are not images")
}

func TestOutputWithAKittyImageMarksThePane(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	attachFake(t, f, Options{})

	f.emit(`%output %1 \033_`)
	f.emit(`%output %1 Ga=T,f=100;AAAA\033\134`)
	f.emit(`%output %1 \033_Ga=d,d=A\033\134`)

	f.awaitCommands(t, "set-option -p -t %1 "+imagePaneOption+" 1", 1)
	require.Len(t, f.commandsMatching(imagePaneOption+" 1"), 1)
}

func TestAttachRedrawsWindowsThatDrewImages(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setWindows(
		"@1 1 %1 120 40 0 b25f,120x40,0,0,1 pi",
		"@2 0 %2 120 40 0 b25f,120x40,0,0,2 shell",
	)
	f.setPanes("", "@1 1", "@2 ")
	attachFake(t, f, Options{})

	f.awaitCommands(t, "set-option -w -t @1 -u window-size", 1)
	require.Equal(t, []string{
		"show-options -w -v -q -t @1 window-size",
		"show-options -w -v -A -t @1 window-size",
		"resize-window -t @1 -x 119",
		"set-option -w -t @1 -u window-size",
	}, f.commandsMatching("-t @1"))
}

func TestARedrawRestoresAWindowsOwnSizeOption(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("", "@1 1")
	f.setOption("@1", "largest")
	attachFake(t, f, Options{})

	f.awaitCommands(t, "set-option -w -t @1 window-size largest", 1)
}

// resize-window stores the width it set, and a window whose size is manual
// keeps that width whatever the option is put back to.
func TestARedrawPutsBackTheWidthOfAManualWindow(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("", "@1 1")
	f.setOption(globalOption, "manual")
	attachFake(t, f, Options{})

	f.awaitCommands(t, "set-option -w -t @1 -u window-size", 1)
	require.Equal(t, []string{"resize-window -t @1 -x 119", "resize-window -t @1 -x 120"}, f.commandsMatching("resize-window"))
}

func TestClosingDuringARedrawRestoresTheWindow(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("", "@1 1")
	// tmux answers the narrowing resize only after Close has cancelled the
	// client: the window is narrow, and the cancelled Send must not hide that.
	cancelled := make(chan struct{})
	var holdOnce sync.Once
	f.setOnCommand(func(cmd string) {
		if strings.HasPrefix(cmd, "resize-window -t @1") {
			holdOnce.Do(func() { <-cancelled })
		}
	})
	client := attachFake(t, f, Options{})
	f.awaitCommands(t, "resize-window -t @1", 1)

	closed := make(chan error, 1)
	go func() { closed <- client.Close(t.Context()) }()
	<-client.lifeCtx.Done()
	close(cancelled)
	require.NoError(t, <-closed)

	sent := f.sentCommands()
	restore := slices.Index(sent, "set-option -w -t @1 -u window-size")
	require.NotEqual(t, -1, restore, "the window's size option is put back")
	require.Less(t, restore, slices.Index(sent, "detach"), "before the detach that ends the gateway")
}

func TestAttachLeavesWindowsWithoutImagesAlone(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("", "@1 ")
	attachFake(t, f, Options{})

	f.awaitCommands(t, "list-panes -s", 1)
	require.Empty(t, f.commandsMatching("resize-window"))
}
