package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameRepositoryRunsRepeatsOfOneRepositoryTogether(t *testing.T) {
	repos := []resolvedRepo{
		{dir: "app", remote: "git@github.com:acme/app.git"},
		{dir: "api", remote: "https://github.com/acme/api"},
		{dir: "app-2", remote: "https://github.com/acme/app"},
	}
	assert.Equal(t, [][]int{{0, 2}, {1}}, sameRepository(repos))
}
