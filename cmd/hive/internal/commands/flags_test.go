package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlagsResolvedLogFile(t *testing.T) {
	tests := []struct {
		name string
		flag Flags
		want string
	}{
		{name: "default", flag: Flags{DataDir: "data"}, want: filepath.Join("data", "hive.log")},
		{name: "override", flag: Flags{DataDir: "data", LogFile: "custom.log"}, want: "custom.log"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.flag.ResolvedLogFile())
		})
	}
}
