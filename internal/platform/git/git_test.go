package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractHost(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"git@github.com:hay-kot/hive.git", "github.com"},
		{"https://github.com/hay-kot/hive.git", "github.com"},
		{"git@gitea.example.com:owner/repo.git", "gitea.example.com"},
		{"https://gitea.example.com/owner/repo", "gitea.example.com"},
		{"ssh://git@git.example.com:2222/owner/repo.git", "git.example.com"},
		{"https://git.example.com:8443/owner/repo", "git.example.com"},
		{"git://github.com/owner/repo.git", "github.com"},
		{"https://user:token@git.example.com/owner/repo", "git.example.com"},
		{"ssh://git.example.com/owner/repo", "git.example.com"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			assert.Equal(t, tt.want, ExtractHost(tt.remote), "ExtractHost(%q)", tt.remote)
		})
	}
}

func TestExtractOwnerRepo(t *testing.T) {
	tests := []struct {
		remote    string
		wantOwner string
		wantRepo  string
	}{
		{"git@github.com:hay-kot/hive.git", "hay-kot", "hive"},
		{"https://github.com/hay-kot/hive.git", "hay-kot", "hive"},
		{"git@github.com:hay-kot/hive", "hay-kot", "hive"},
		{"https://github.com/hay-kot/hive", "hay-kot", "hive"},
		{"git@gitlab.com:org/subgroup/repo.git", "subgroup", "repo"},
		{"https://gitlab.com/org/subgroup/repo.git", "subgroup", "repo"},
		{"invalid", "", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			owner, repo := ExtractOwnerRepo(tt.remote)
			assert.Equal(t, tt.wantOwner, owner, "ExtractOwnerRepo(%q) owner mismatch", tt.remote)
			assert.Equal(t, tt.wantRepo, repo, "ExtractOwnerRepo(%q) repo mismatch", tt.remote)
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		remote   string
		wantRepo string
	}{
		{"git@github.com:hay-kot/hive.git", "hive"},
		{"https://github.com/hay-kot/hive.git", "hive"},
		{"git@github.com:hay-kot/hive", "hive"},
		{"https://github.com/hay-kot/hive", "hive"},
	}

	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			repo := ExtractRepoName(tt.remote)
			assert.Equal(t, tt.wantRepo, repo, "ExtractRepoName(%q) = %q, want %q", tt.remote, repo, tt.wantRepo)
		})
	}
}
