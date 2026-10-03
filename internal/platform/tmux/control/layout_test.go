package tmuxcc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func singlePaneLayout(pane string, width, height int) Layout {
	return Layout{Pane: pane, Width: width, Height: height}
}

func TestParseLayout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want Layout
	}{
		{
			name: "one pane",
			in:   "b25f,120x40,0,0,1",
			want: Layout{Pane: "%1", Width: 120, Height: 40},
		},
		{
			// Recorded from tmux 3.7b: split-window -h, then -v on the right pane.
			name: "left-right holding a top-bottom",
			in:   "95e4,120x40,0,0{60x40,0,0,0,59x40,61,0[59x20,61,0,1,59x19,61,21,2]}",
			want: Layout{
				Split: SplitLeftRight, Width: 120, Height: 40,
				Cells: []Layout{
					{Pane: "%0", Width: 60, Height: 40},
					{
						Split: SplitTopBottom, Width: 59, Height: 40, X: 61,
						Cells: []Layout{
							{Pane: "%1", Width: 59, Height: 20, X: 61},
							{Pane: "%2", Width: 59, Height: 19, X: 61, Y: 21},
						},
					},
				},
			},
		},
		{
			name: "three side by side",
			in:   "c74c,120x40,0,0{30x40,0,0,0,44x40,31,0,1,44x40,76,0,3}",
			want: Layout{
				Split: SplitLeftRight, Width: 120, Height: 40,
				Cells: []Layout{
					{Pane: "%0", Width: 30, Height: 40},
					{Pane: "%1", Width: 44, Height: 40, X: 31},
					{Pane: "%3", Width: 44, Height: 40, X: 76},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLayout(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.True(t, got.Equal(tc.want))
			_, body, _ := splitChecksum(tc.in)
			require.Equal(t, body, got.String())
		})
	}
}

func splitChecksum(s string) (string, string, bool) {
	for i := range len(s) {
		if s[i] == ',' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func TestParseLayoutRefusesWhatItCannotPlace(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"",
		"b25f",
		"b25f,120x40,0,0",
		"b25f,120x40,0,0,",
		"b25f,120x40,0,0{60x40,0,0,0",
		"b25f,120x40,0,0{60x40,0,0,0]",
		"b25f,120x40,0,0{}",
		"b25f,120x40,0,0,1 trailing",
		"b25f,120x40,0,0,1,2",
		"b25f,120x4000000000,0,0,1",
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := ParseLayout(in)
			require.ErrorIs(t, err, errLayout)
		})
	}
}

func TestParseLayoutBoundsItsDepth(t *testing.T) {
	t.Parallel()

	nested := func(levels int) string {
		return "b25f," + strings.Repeat("120x40,0,0{", levels) + "120x40,0,0,1" + strings.Repeat("}", levels)
	}
	_, err := ParseLayout(nested(maxLayoutDepth))
	require.NoError(t, err)
	_, err = ParseLayout(nested(maxLayoutDepth + 1))
	require.ErrorIs(t, err, errLayout)
}

func TestLayoutLeavesAndPanes(t *testing.T) {
	t.Parallel()

	l, err := ParseLayout("95e4,120x40,0,0{60x40,0,0,0,59x40,61,0[59x20,61,0,1,59x19,61,21,2]}")
	require.NoError(t, err)
	require.Equal(t, []string{"%0", "%1", "%2"}, l.Panes(), "left to right, then top to bottom")
	require.Len(t, l.Leaves(), 3)
	require.Equal(t, Layout{Pane: "%2", Width: 59, Height: 19, X: 61, Y: 21}, l.Leaves()[2])

	require.Empty(t, Layout{}.Panes(), "a window whose layout has not been read has no panes to offer")
}

func TestLayoutEqual(t *testing.T) {
	t.Parallel()

	a, err := ParseLayout("c74c,120x40,0,0{30x40,0,0,0,89x40,31,0,1}")
	require.NoError(t, err)
	b, err := ParseLayout("f91d,120x40,0,0{60x40,0,0,0,59x40,61,0,1}")
	require.NoError(t, err)
	same, err := ParseLayout("0000,120x40,0,0{30x40,0,0,0,89x40,31,0,1}")
	require.NoError(t, err)
	require.False(t, a.Equal(b), "a moved divider is a different layout")
	require.True(t, a.Equal(same), "the checksum is not part of the tree")
	require.False(t, a.Equal(Layout{}))
}
