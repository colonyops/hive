package commands

import "path/filepath"

// EffectiveLogFile returns flags.LogFile if non-empty, otherwise
// filepath.Join(flags.DataDir, "hive.log"). Mirrors the default applied by
// the global Before hook in main.go so a child process (e.g. timer-fire)
// can inherit the exact same path.
func EffectiveLogFile(flags *Flags) string {
	if flags.LogFile != "" {
		return flags.LogFile
	}
	return filepath.Join(flags.DataDir, "hive.log")
}
