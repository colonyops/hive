package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor_GetDiff(t *testing.T) {
	const sampleDiff = `diff --git a/file.go b/file.go
index abc123..def456 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main

 func main() {
+	fmt.Println("hello")
 }`

	tests := []struct {
		name    string
		opts    DiffOptions
		want    string
		wantErr bool
	}{
		{
			name: "uncommitted changes",
			opts: DiffOptions{
				Mode: DiffUncommitted,
			},
			want: sampleDiff,
		},
		{
			name: "staged changes",
			opts: DiffOptions{
				Mode: DiffStaged,
			},
			want: sampleDiff,
		},
		{
			name: "branch comparison",
			opts: DiffOptions{
				Mode:       DiffBranch,
				BaseBranch: "main",
			},
			want: sampleDiff,
		},
		{
			name: "branch comparison without base branch",
			opts: DiffOptions{
				Mode: DiffBranch,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExecutor{
				runDirFunc: func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
					// Verify the git command arguments match expected mode
					switch tt.opts.Mode {
					case DiffUncommitted:
						assert.Equal(t, []string{"diff", "HEAD"}, args)
					case DiffStaged:
						assert.Equal(t, []string{"diff", "--staged"}, args)
					case DiffBranch:
						if tt.opts.BaseBranch != "" {
							assert.Equal(t, []string{"diff", tt.opts.BaseBranch + "...HEAD"}, args)
						}
					}

					return []byte(sampleDiff), nil
				},
			}

			e := NewExecutor("git", mock)
			got, err := e.GetDiff(context.Background(), "/test/dir", tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDescribeDiffMode(t *testing.T) {
	tests := []struct {
		name string
		opts DiffOptions
		want string
	}{
		{
			name: "uncommitted",
			opts: DiffOptions{Mode: DiffUncommitted},
			want: "uncommitted changes",
		},
		{
			name: "staged",
			opts: DiffOptions{Mode: DiffStaged},
			want: "staged changes",
		},
		{
			name: "branch",
			opts: DiffOptions{
				Mode:       DiffBranch,
				BaseBranch: "main",
			},
			want: "changes vs main",
		},
		{
			name: "unknown mode",
			opts: DiffOptions{Mode: DiffMode(999)},
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DescribeDiffMode(tt.opts)
			assert.Equal(t, tt.want, got)
		})
	}
}
