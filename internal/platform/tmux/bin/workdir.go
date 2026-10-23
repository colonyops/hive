package tmuxbin

import "os"

// WorkDir is the working directory to exec tmux in: $HOME, or / when $HOME is
// unset or not a directory.
//
// A tmux client that finds no server starts one, and the server keeps the
// client's working directory for its whole life. Hive deletes session
// checkouts on recycle and prune, and once the server's directory is gone tmux
// 3.8 spawns every new pane in it and ignores -c: spawn_pane only changes to
// the pane's directory when its own getcwd succeeds. So no hive process may
// exec tmux from its own working directory, which is often a checkout.
func WorkDir() string {
	return workDir(os.Getenv("HOME"))
}

func workDir(home string) string {
	if home != "" {
		if info, err := os.Stat(home); err == nil && info.IsDir() {
			return home
		}
	}
	return "/"
}
