package gitea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
)

const testCredential = Provider + "/git.example.com-octocat"

func TestConfigValidateAcceptsTheTwoKinds(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&Config{Credential: testCredential, Kind: KindSearch}).Validate())
	require.NoError(t, (&Config{Credential: testCredential, Kind: KindNotifications}).Validate())
}

func TestConfigValidateRejectsBadCredentials(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]*Config{
		"missing":         {Kind: KindSearch},
		"other provider":  {Credential: "github/octocat", Kind: KindSearch},
		"not a ref":       {Credential: "octocat", Kind: KindSearch},
		"no account half": {Credential: "gitea/", Kind: KindSearch},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, config.Validate())
		})
	}
}

func TestConfigValidateRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, (&Config{Credential: testCredential}).Validate(), "kind is required")
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: "issues"}).Validate(), "unknown kind")
}

// Gitea answers an unknown state or type with an unfiltered page rather than an
// error, so a typo that reached the API would read as "these are all my open
// pull requests". Every enumerated value is checked here instead.
func TestConfigValidateRejectsValuesGiteaWouldSilentlyIgnore(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Items: "prs"}).Validate(), "unknown items")
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, State: "merged"}).Validate(), "unknown state")
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Involving: []string{"watching"}}).Validate(), "unknown involving")
}

// The labels parameter is comma-joined with no escaping, so a label containing
// a comma would silently become two nonexistent filters.
func TestConfigValidateRejectsUnsendableLabels(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Labels: []string{" "}}).Validate(), "blank")
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Labels: []string{"needs: design, ux"}}).Validate(), "comma")
}

func TestConfigValidateRejectsDuplicateInvolving(t *testing.T) {
	t.Parallel()

	config := &Config{Credential: testCredential, Kind: KindSearch, Involving: []string{InvolvingAssigned, InvolvingAssigned}}
	assert.ErrorContains(t, config.Validate(), "listed twice")
}

// The inbox cannot be filtered server-side, so a notifications node carrying
// filters would look filtered and return everything. Failing says so.
func TestConfigValidateRejectsSearchFiltersOnNotifications(t *testing.T) {
	t.Parallel()

	config := &Config{Credential: testCredential, Kind: KindNotifications, State: StateOpen, Owner: "acme"}
	err := config.Validate()
	require.Error(t, err)
	require.ErrorContains(t, err, "state")
	assert.ErrorContains(t, err, "owner")
}

func TestConfigValidateEnforcesPerKindLimits(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&Config{Credential: testCredential, Kind: KindSearch, Limit: maxSearchLimit}).Validate())
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Limit: maxSearchLimit + 1}).Validate(), "search cap")
	require.NoError(t, (&Config{Credential: testCredential, Kind: KindNotifications, Limit: maxNotificationsLimit}).Validate())
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindNotifications, Limit: maxNotificationsLimit + 1}).Validate(), "notifications page cap")
	require.ErrorContains(t, (&Config{Credential: testCredential, Kind: KindSearch, Limit: -1}).Validate(), "negative")
}

func TestSearchRequestsDefaultsToOneOpenSearch(t *testing.T) {
	t.Parallel()

	requests := (&Config{Credential: testCredential, Kind: KindSearch}).searchRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, giteaclient.SearchRequest{State: StateOpen, Limit: defaultLimit}, requests[0])
}

// Gitea intersects its involvement parameters, so a union is one request each.
// Sending them together returns items matching *all* of them, which is empty in
// practice — the bug this shape exists to avoid.
func TestSearchRequestsAreOnePerInvolvement(t *testing.T) {
	t.Parallel()

	config := &Config{
		Credential: testCredential,
		Kind:       KindSearch,
		Items:      ItemsPulls,
		State:      StateAll,
		Involving:  []string{InvolvingReviewRequested, InvolvingAssigned},
		Owner:      "acme",
		Labels:     []string{"bug"},
		Text:       "checkout",
		Limit:      10,
	}

	requests := config.searchRequests()
	require.Len(t, requests, 2)
	assert.Equal(t, giteaclient.InvolvementReviewRequested, requests[0].Involvement)
	assert.Equal(t, giteaclient.InvolvementAssigned, requests[1].Involvement)
	for _, req := range requests {
		assert.Equal(t, "pulls", req.Type)
		assert.Equal(t, StateAll, req.State)
		assert.Equal(t, "acme", req.Owner)
		assert.Equal(t, []string{"bug"}, req.Labels)
		assert.Equal(t, "checkout", req.Text)
		assert.Equal(t, 10, req.Limit)
	}
}

// "all" is the connector's word for "do not filter", which the client spells
// by omitting the parameter: Gitea ignores a type it does not recognize, so
// sending type=all would only work by accident.
func TestSearchRequestsSpellAllItemsAsNoTypeFilter(t *testing.T) {
	t.Parallel()

	requests := (&Config{Credential: testCredential, Kind: KindSearch, Items: ItemsAll}).searchRequests()
	require.Len(t, requests, 1)
	assert.Empty(t, requests[0].Type)
}
