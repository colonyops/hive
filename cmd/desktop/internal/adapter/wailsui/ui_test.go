package wailsui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSingleInstanceID(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		dataDir string
		want    string
	}{
		{
			name:    "hash starts with a digit",
			dataDir: "/home/darren/.local/share/hive",
			want:    "com.hivedesktop.app.data_8cb7219363f01ee5",
		},
		{
			name:    "hash starts with a letter",
			dataDir: "/tmp/hive",
			want:    "com.hivedesktop.app.data_ada40432028a5122",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := singleInstanceID(tt.dataDir)
			assert.Equal(t, tt.want, got)
			assert.Regexp(t, `^[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z_][A-Za-z0-9_-]*)+$`, got)
		})
	}
}
