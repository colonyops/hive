//go:build windows

package tmuxexec

import (
	"context"
	"fmt"
)

func (r execRunner) lockLaunch(context.Context, string) (func() error, error) {
	return nil, fmt.Errorf("tmux launches are not supported on Windows")
}
