//go:build linux

package process

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTpgid(t *testing.T) {
	tests := []struct {
		name    string
		stat    string
		want    int
		wantErr bool
	}{
		{
			name: "simple comm",
			stat: "1234 (bash) S 1233 1234 1234 34818 5678 ...",
			want: 5678,
		},
		{
			name: "comm with spaces",
			stat: "1234 (my process) S 1233 1234 1234 34818 5678 ...",
			want: 5678,
		},
		{
			name: "comm with parens",
			stat: "1234 (foo(bar)) S 1233 1234 1234 34818 9999 ...",
			want: 9999,
		},
		{
			name:    "malformed no paren",
			stat:    "1234 bash S 1233 1234",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTpgid(tt.stat)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
