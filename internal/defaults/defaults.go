// Package defaults embeds sample configuration files that hive can provide
// to users during setup or on request. The merge/write logic lives in callers;
// this package only supplies the raw content.
package defaults

import _ "embed"

// HiveConfig is the sample hive config.yaml with annotated defaults.
//
//go:embed config.yaml
var HiveConfig []byte

// TmuxConfig is a sample tmux.conf tuned for use with hive.
//
//go:embed tmux.conf
var TmuxConfig []byte
