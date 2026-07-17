package git

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffStats(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantAdd int
		wantDel int
		wantErr bool
	}{
		{
			name:    "empty output means clean",
			output:  "",
			wantAdd: 0,
			wantDel: 0,
		},
		{
			name:    "whitespace only means clean",
			output:  "   \n  ",
			wantAdd: 0,
			wantDel: 0,
		},
		{
			name:    "insertions and deletions",
			output:  " 3 files changed, 10 insertions(+), 5 deletions(-)",
			wantAdd: 10,
			wantDel: 5,
		},
		{
			name:    "insertions only",
			output:  " 1 file changed, 25 insertions(+)",
			wantAdd: 25,
			wantDel: 0,
		},
		{
			name:    "deletions only",
			output:  " 2 files changed, 15 deletions(-)",
			wantAdd: 0,
			wantDel: 15,
		},
		{
			name:    "single insertion",
			output:  " 1 file changed, 1 insertion(+)",
			wantAdd: 1,
			wantDel: 0,
		},
		{
			name:    "single deletion",
			output:  " 1 file changed, 1 deletion(-)",
			wantAdd: 0,
			wantDel: 1,
		},
		{
			name:    "large numbers",
			output:  " 50 files changed, 1234 insertions(+), 567 deletions(-)",
			wantAdd: 1234,
			wantDel: 567,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, del, err := parseDiffStats(tt.output)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAdd, add, "additions mismatch")
			assert.Equal(t, tt.wantDel, del, "deletions mismatch")
		})
	}
}

// mockExecutor is a simple mock for testing git executor methods.
type mockExecutor struct {
	runDirFunc func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error)
}

func (m *mockExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	return nil, nil
}

func (m *mockExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	if m.runDirFunc != nil {
		return m.runDirFunc(ctx, dir, cmd, args...)
	}
	return nil, nil
}

func (m *mockExecutor) RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	return nil
}

func (m *mockExecutor) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	return nil
}

func TestExecutor_Branch(t *testing.T) {
	tests := []struct {
		name        string
		branchOut   string
		revParseOut string
		want        string
		wantErr     bool
	}{
		{
			name:      "normal branch",
			branchOut: "main\n",
			want:      "main",
		},
		{
			name:      "feature branch with slashes",
			branchOut: "feature/my-feature\n",
			want:      "feature/my-feature",
		},
		{
			name:        "detached HEAD",
			branchOut:   "\n",
			revParseOut: "abc1234\n",
			want:        "abc1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockExecutor{
				runDirFunc: func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
					callCount++
					if callCount == 1 {
						// First call is branch --show-current
						return []byte(tt.branchOut), nil
					}
					// Second call is rev-parse --short HEAD
					return []byte(tt.revParseOut), nil
				},
			}

			e := NewExecutor("git", mock)
			got, err := e.Branch(context.Background(), "/test/dir")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExecutor_DefaultBranch(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{
			name:   "main branch",
			output: "origin/main\n",
			want:   "main",
		},
		{
			name:   "master branch",
			output: "origin/master\n",
			want:   "master",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExecutor{
				runDirFunc: func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
					return []byte(tt.output), nil
				},
			}

			e := NewExecutor("git", mock)
			got, err := e.DefaultBranch(context.Background(), "/test/dir")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExecutor_WorktreeRemove(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		wantCalls  int
		wantBranch bool
	}{
		{name: "removes worktree and branch", branch: "hive/session", wantCalls: 2, wantBranch: true},
		{name: "removes worktree without branch metadata", wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]string
			mock := &mockExecutor{
				runDirFunc: func(_ context.Context, dir, cmd string, args ...string) ([]byte, error) {
					require.Equal(t, "/bare", dir)
					require.Equal(t, "git", cmd)
					calls = append(calls, args)
					return nil, nil
				},
			}

			e := NewExecutor("git", mock)
			err := e.WorktreeRemove(context.Background(), "/bare", "/worktree", tt.branch)

			require.NoError(t, err)
			require.Len(t, calls, tt.wantCalls)
			assert.Equal(t, []string{"worktree", "remove", "--force", "/worktree"}, calls[0])
			if tt.wantBranch {
				assert.Equal(t, []string{"branch", "-D", tt.branch}, calls[1])
			}
		})
	}
}

func TestExecutor_Fetch(t *testing.T) {
	tests := []struct {
		name       string
		bareOut    string
		wantFetch  []string
		wantErr    bool
		fetchError error
	}{
		{
			name:      "normal repository fetches origin",
			bareOut:   "false\n",
			wantFetch: []string{"fetch", "origin"},
		},
		{
			name:      "bare repository fetches default branch ref",
			bareOut:   "true\n",
			wantFetch: []string{"fetch", "origin", "+refs/heads/main:refs/heads/main", "--prune"},
		},
		{
			name:       "fetch error is returned",
			bareOut:    "true\n",
			wantFetch:  []string{"fetch", "origin", "+refs/heads/main:refs/heads/main", "--prune"},
			wantErr:    true,
			fetchError: fmt.Errorf("network down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockExecutor{
				runDirFunc: func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
					callCount++
					switch callCount {
					case 1:
						assert.Equal(t, "/repo", dir)
						assert.Equal(t, []string{"--no-optional-locks", "rev-parse", "--is-bare-repository"}, args)
						return []byte(tt.bareOut), nil
					case 2:
						assert.Equal(t, "/repo", dir)
						if strings.TrimSpace(tt.bareOut) == "true" {
							assert.Equal(t, []string{"--no-optional-locks", "symbolic-ref", "HEAD", "--short"}, args)
							return []byte("main\n"), nil
						}
						assert.Equal(t, tt.wantFetch, args)
						return nil, tt.fetchError
					case 3:
						assert.Equal(t, "/repo", dir)
						assert.Equal(t, tt.wantFetch, args)
						return nil, tt.fetchError
					default:
						return nil, fmt.Errorf("unexpected call %d", callCount)
					}
				},
			}

			e := NewExecutor("git", mock)
			err := e.Fetch(context.Background(), "/repo")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "git fetch")
				return
			}
			require.NoError(t, err)
			if strings.TrimSpace(tt.bareOut) == "true" {
				assert.Equal(t, 3, callCount)
			} else {
				assert.Equal(t, 2, callCount)
			}
		})
	}
}

func TestExecutor_DiffStats(t *testing.T) {
	tests := []struct {
		name          string
		defaultBranch string
		diffOutput    string
		wantAdd       int
		wantDel       int
		wantErr       bool
	}{
		{
			name:          "with changes against main",
			defaultBranch: "origin/main\n",
			diffOutput:    " 2 files changed, 10 insertions(+), 5 deletions(-)\n",
			wantAdd:       10,
			wantDel:       5,
		},
		{
			name:          "no changes",
			defaultBranch: "origin/main\n",
			diffOutput:    "",
			wantAdd:       0,
			wantDel:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockExecutor{
				runDirFunc: func(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
					callCount++
					if callCount == 1 {
						// First call is symbolic-ref for default branch
						return []byte(tt.defaultBranch), nil
					}
					// Second call is diff --shortstat
					return []byte(tt.diffOutput), nil
				},
			}

			e := NewExecutor("git", mock)
			add, del, err := e.DiffStats(context.Background(), "/test/dir")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAdd, add)
			assert.Equal(t, tt.wantDel, del)
		})
	}
}
