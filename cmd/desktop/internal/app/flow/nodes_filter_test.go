package flow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGithubFilterValidatesPullRequestStates(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&GithubFilterConfig{CI: []string{"passing", "none"}, Review: []string{"approved", "draft"}}).Validate(nil))
	require.ErrorContains(t, (&GithubFilterConfig{CI: []string{"green"}}).Validate(nil), "unknown CI state")
	require.ErrorContains(t, (&GithubFilterConfig{Review: []string{"accepted"}}).Validate(nil), "unknown review state")
}
