package osopen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCmd(t *testing.T) {
	cases := []struct {
		goos     string
		path     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "/x/y.log", "open", []string{"/x/y.log"}},
		{"linux", "/x/y.log", "xdg-open", []string{"/x/y.log"}},
		{"freebsd", "/x/y.log", "xdg-open", []string{"/x/y.log"}},
		{"windows", `C:\x\y.log`, "cmd", []string{"/c", "start", "", `C:\x\y.log`}},
		{"plan9", "/x/y.log", "", nil},
	}
	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			name, args := openCmd(c.goos, c.path)
			require.Equal(t, c.wantName, name)
			require.Equal(t, c.wantArgs, args)
		})
	}
}

func TestRevealCmd(t *testing.T) {
	cases := []struct {
		goos     string
		path     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "/x/y.log", "open", []string{"-R", "/x/y.log"}},
		{"linux", "/x/y.log", "xdg-open", []string{"/x"}},
		{"windows", `C:\x\y.log`, "explorer", []string{`/select,C:\x\y.log`}},
		{"plan9", "/x/y.log", "", nil},
	}
	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			name, args := revealCmd(c.goos, c.path)
			require.Equal(t, c.wantName, name)
			require.Equal(t, c.wantArgs, args)
		})
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	require.Error(t, Open(""))
	require.Error(t, Reveal(""))
}
