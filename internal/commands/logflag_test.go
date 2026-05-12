package commands_test

import (
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/commands"
)

func TestEffectiveLogFile(t *testing.T) {
	tests := []struct {
		name string
		in   commands.Flags
		want string
	}{
		{name: "explicit log file overrides data dir", in: commands.Flags{LogFile: "/tmp/custom.log", DataDir: "/var/data"}, want: "/tmp/custom.log"},
		{name: "default is data dir + hive.log", in: commands.Flags{DataDir: "/var/data"}, want: filepath.Join("/var/data", "hive.log")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commands.EffectiveLogFile(&tt.in)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
