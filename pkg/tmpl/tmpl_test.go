package tmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		data    any
		want    string
		wantErr bool
	}{
		{
			name: "simple substitution",
			tmpl: "hello {{ .Name }}",
			data: map[string]string{"Name": "world"},
			want: "hello world",
		},
		{
			name: "multiple variables",
			tmpl: `cd "{{ .Path }}" && echo "{{ .Prompt }}"`,
			data: map[string]string{
				"Path":   "/tmp/session",
				"Prompt": "implement feature X",
			},
			want: `cd "/tmp/session" && echo "implement feature X"`,
		},
		{
			name: "struct data",
			tmpl: "{{ .Name }} at {{ .Path }}",
			data: struct {
				Name string
				Path string
			}{Name: "test", Path: "/tmp"},
			want: "test at /tmp",
		},
		{
			name: "no variables",
			tmpl: "static string",
			data: nil,
			want: "static string",
		},
		{
			name:    "missing key errors",
			tmpl:    "{{ .Missing }}",
			data:    map[string]string{"Name": "test"},
			wantErr: true,
		},
		{
			name:    "invalid template syntax",
			tmpl:    "{{ .Name }",
			data:    map[string]string{"Name": "test"},
			wantErr: true,
		},
		{
			name: "empty value is valid",
			tmpl: "prefix{{ .Name }}suffix",
			data: map[string]string{"Name": ""},
			want: "prefixsuffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
