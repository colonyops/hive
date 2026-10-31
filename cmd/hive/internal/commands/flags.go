package commands

import "github.com/colonyops/hive/internal/config"

type Flags struct {
	LogLevel     string
	LogFile      string
	ConfigPath   string
	DataDir      string
	ProfilerPort int
}

// ResolvedLogFile returns the log file path in use: the explicit --log-file
// value when set, otherwise <datadir>/hive.log.
func (f *Flags) ResolvedLogFile() string {
	if f.LogFile != "" {
		return f.LogFile
	}
	return config.LogFilePath(f.DataDir)
}
