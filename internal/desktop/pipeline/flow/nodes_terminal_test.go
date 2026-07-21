package flow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFeedConfig_Validate(t *testing.T) {
	t.Run("empty icon and description are allowed", func(t *testing.T) {
		require.NoError(t, (&FeedConfig{}).Validate(nil))
	})

	t.Run("supported icon is allowed", func(t *testing.T) {
		require.NoError(t, (&FeedConfig{Icon: "sparkles"}).Validate(nil))
	})

	t.Run("unsupported icon is rejected", func(t *testing.T) {
		err := (&FeedConfig{Icon: "not-an-icon"}).Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "icon")
	})

	t.Run("description over the cap is rejected", func(t *testing.T) {
		err := (&FeedConfig{Description: strings.Repeat("x", feedDescriptionMaxLen+1)}).Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "description")
	})

	t.Run("description at the cap is allowed", func(t *testing.T) {
		require.NoError(t, (&FeedConfig{Description: strings.Repeat("x", feedDescriptionMaxLen)}).Validate(nil))
	})
}

func TestFeedConfig_RoundTrip(t *testing.T) {
	n := Node{
		ID:     "team-feed",
		Type:   "feed",
		Name:   "Team feed",
		Config: &FeedConfig{Icon: "sparkles", Description: "PRs the triage bot flagged for the team."},
	}

	jsonData, err := json.Marshal(n)
	require.NoError(t, err)
	var fromJSON Node
	require.NoError(t, json.Unmarshal(jsonData, &fromJSON))
	assert.Equal(t, n, fromJSON)

	yamlData, err := yaml.Marshal(n)
	require.NoError(t, err)
	var fromYAML Node
	require.NoError(t, yaml.Unmarshal(yamlData, &fromYAML))
	assert.Equal(t, n, fromYAML)
}

func TestFeedConfig_OmitsEmptyFields(t *testing.T) {
	data, err := json.Marshal(Node{ID: "sink", Type: "feed", Config: &FeedConfig{}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "icon")
	assert.NotContains(t, string(data), "description")
}
