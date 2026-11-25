package engineops

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalPath(t *testing.T) {
	t.Parallel()
	for _, remote := range []string{"/srv/repo.git", "./repo", "../repo", "~/repo", "file:///srv/repo.git"} {
		assert.True(t, isLocalPath(remote), remote)
	}
	for _, remote := range []string{"git@github.com:colonyops/hive.git", "https://github.com/colonyops/hive.git", "ssh://git@host/repo.git"} {
		assert.False(t, isLocalPath(remote), remote)
	}
}
