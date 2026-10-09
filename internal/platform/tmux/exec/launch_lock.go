package tmuxexec

import "context"

type launchLocker interface {
	lockLaunch(context.Context, string) (func() error, error)
}

func (c *Client) withLaunchLock(ctx context.Context, name string, run func() error) (err error) {
	locker, ok := c.runner.(launchLocker)
	if !ok {
		return run()
	}
	release, err := locker.lockLaunch(ctx, name)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				c.log.Warn().Err(releaseErr).Msg("failed to release tmux launch lock")
			}
		}
	}()
	return run()
}
