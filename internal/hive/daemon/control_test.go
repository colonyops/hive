package daemon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoTmuxServer(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"tmux list-panes: no server running on /tmp/tmux-1000/default: exit status 1", true},
		{"tmux list-panes: error connecting to /tmp/tmux-1000/default (No such file or directory)", true},
		{`tmux list-panes: exec: "tmux": executable file not found in $PATH`, false},
		{"tmux list-panes: signal: killed", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, noTmuxServer(errors.New(tt.text)), tt.text)
	}
}
