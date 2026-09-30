package activity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstructors(t *testing.T) {
	cases := []struct {
		name     string
		event    Event
		category Category
		severity Severity
	}{
		{"refresh-failed", RefreshFailed("s", "boom"), CategoryRefresh, SeverityError},
		{"session", SessionCreated("review-pr-2841", "sonnet", "hive/core"), CategorySession, SeveritySuccess},
		{"auto-action", AutoAction("Triage", "triage.default", "#4820"), CategoryAutoAction, SeverityAuto},
		{"action", ActionRun("Reproduce", "exit 0"), CategoryAction, SeveritySuccess},
		{"action-failed", ActionFailed("Reproduce", "exit 1"), CategoryAction, SeverityError},
		{"config", ConfigReloaded("actions.yml", 6), CategoryConfig, SeverityInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.category, tc.event.Category)
			require.Equal(t, tc.severity, tc.event.Severity)
			require.NotEmpty(t, tc.event.Title)
		})
	}
}

func TestEventWithLinkPreservesOtherMetadata(t *testing.T) {
	event := Event{Metadata: map[string]string{"retry": "session-create"}}.WithLink(Link{
		URL: "https://github.com/acme/api/pull/12",
		Item: &ItemLink{
			ProfileID:   "triage",
			SourceKind:  "github",
			SourceScope: "acme/api",
			ExternalID:  "acme/api#12",
		},
	})

	require.Equal(t, map[string]string{
		"retry":                     "session-create",
		MetadataLinkURL:             "https://github.com/acme/api/pull/12",
		MetadataLinkItemProfileID:   "triage",
		MetadataLinkItemSourceKind:  "github",
		MetadataLinkItemSourceScope: "acme/api",
		MetadataLinkItemExternalID:  "acme/api#12",
	}, event.Metadata)
}

func TestEventWithEmptyLinkLeavesMetadataNil(t *testing.T) {
	require.Nil(t, (Event{}).WithLink(Link{}).Metadata)
}

func TestRecordInputEvent(t *testing.T) {
	ev, err := RecordInput{Title: "Profile deleted", Severity: "success"}.Event()
	require.NoError(t, err)
	require.Equal(t, CategorySystem, ev.Category, "empty category defaults to system")
	require.Equal(t, SeveritySuccess, ev.Severity)

	_, err = RecordInput{Severity: "success"}.Event()
	require.Error(t, err, "missing title is rejected")

	_, err = RecordInput{Title: "x", Category: "bogus"}.Event()
	require.Error(t, err, "invalid category is rejected")
}
