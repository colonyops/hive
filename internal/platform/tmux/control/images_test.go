package tmuxcc

import (
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

	f.awaitCommands(t, "refresh-client -C @1:", 2)
	require.Equal(t, []string{"refresh-client -C @1:119x40", "refresh-client -C @1:"}, f.commandsMatching("refresh-client -C @"))
}

func TestAttachLeavesWindowsWithoutImagesAlone(t *testing.T) {
	t.Parallel()

	f := newFakeTmux(t, "hive-demo")
	f.setPanes("", "@1 ")
	attachFake(t, f, Options{})

	f.awaitCommands(t, "list-panes -s", 1)
	require.Empty(t, f.commandsMatching("refresh-client -C @"))
}
