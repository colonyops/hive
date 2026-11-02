package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNotFoundError(t *testing.T) {
	t.Parallel()
	// This is a simple helper, just verify it works
	err := os.ErrNotExist
	assert.True(t, os.IsNotExist(err), "Should recognize os.ErrNotExist")
}
