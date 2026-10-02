package dispatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
)

type flowListerTest struct{ flows []flow.Flow }

func (f flowListerTest) List() []flow.Flow { return f.flows }

type actionListerTest struct{ actions map[string]actions.Action }

func (a actionListerTest) Get(id string) (actions.Action, bool) {
	action, ok := a.actions[id]
	return action, ok
}

func notifyFlows() flowListerTest {
	silent := false
	return flowListerTest{flows: []flow.Flow{{
		ID: "triage",
		Nodes: []flow.Node{
			{ID: "src", Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat"})},
			{ID: "tell-me", Type: "notify", Name: "Tell me", Config: &flow.NotifyConfig{
				Title: "{{ .Payload.repo }}", Body: "{{ .Payload.title }}", Severity: "warning", Sound: &silent,
			}},
			{ID: "bare", Type: "notify", Config: &flow.NotifyConfig{Title: "hi"}},
		},
	}}}
}

func TestFlowActions_SynthesizesFromTheFlowNode(t *testing.T) {
	lister := NewFlowActions(notifyFlows(), actionListerTest{})

	action, ok := lister.Get(models.NotifyActionID("triage/tell-me"))
	require.True(t, ok)
	assert.Equal(t, ActionTypeNotify, action.Type)
	assert.Equal(t, "Tell me", action.Label)
	assert.Equal(t, &NotifyActionConfig{
		Title:    "{{ .Payload.repo }}",
		Body:     "{{ .Payload.title }}",
		Severity: "warning",
		Sound:    false,
		Cooldown: flow.NotifyCooldownDefault,
	}, action.Config)
}

// A node with no author-given name still needs a label for the Activity view
// and the jobs list, and its unset optional fields resolve to their defaults.
func TestFlowActions_FillsDefaultsForABareNode(t *testing.T) {
	lister := NewFlowActions(notifyFlows(), actionListerTest{})

	action, ok := lister.Get(models.NotifyActionID("triage/bare"))
	require.True(t, ok)
	assert.Equal(t, "Notify bare", action.Label)
	assert.Equal(t, &NotifyActionConfig{Title: "hi", Severity: flow.NotifySeverityDefault, Sound: true, Cooldown: flow.NotifyCooldownDefault}, action.Config)
}

// The node's own cooldown — including an explicit 0 — is resolved into the
// action config, so the executor never has to know the default.
func TestFlowActions_ProjectsTheResolvedCooldown(t *testing.T) {
	disabled := 0
	flows := flowListerTest{flows: []flow.Flow{{ID: "triage", Nodes: []flow.Node{
		{ID: "eager", Type: "notify", Config: &flow.NotifyConfig{Title: "hi", CooldownSeconds: &disabled}},
	}}}}

	action, ok := NewFlowActions(flows, nil).Get(models.NotifyActionID("triage/eager"))
	require.True(t, ok)
	cfg, ok := action.Config.(*NotifyActionConfig)
	require.True(t, ok)
	assert.Equal(t, time.Duration(0), cfg.Cooldown)
}

func TestFlowActions_DelegatesAuthoredIDs(t *testing.T) {
	authored := actions.Action{ID: "review-pr", Type: "launch-session"}
	lister := NewFlowActions(notifyFlows(), actionListerTest{actions: map[string]actions.Action{"review-pr": authored}})

	action, ok := lister.Get("review-pr")
	require.True(t, ok)
	assert.Equal(t, authored, action)

	_, ok = lister.Get("does-not-exist")
	assert.False(t, ok)
}

// A notify id whose node is gone is unknown, exactly like a deleted
// actions.yml entry: its queued command fails visibly instead of silently
// doing nothing.
func TestFlowActions_UnknownNotifyTargets(t *testing.T) {
	lister := NewFlowActions(notifyFlows(), actionListerTest{})

	for _, id := range []string{
		models.NotifyActionID("triage/deleted"),
		models.NotifyActionID("other-flow/tell-me"),
		models.NotifyActionID("triage/src"), // exists, but is not a notify node
		models.NotifyActionID("no-slash"),
		models.NotifyActionPrefix,
	} {
		_, ok := lister.Get(id)
		assert.False(t, ok, id)
	}
}

func TestNotifyActionID_RoundTrips(t *testing.T) {
	target, ok := models.NotifyActionTarget(models.NotifyActionID("triage/tell-me"))
	require.True(t, ok)
	assert.Equal(t, "triage/tell-me", target)

	// An authored action id is a slug, so it can never look like a notify id.
	_, ok = models.NotifyActionTarget("review-pr")
	assert.False(t, ok)
}

func TestNotifyActionConfig_RequiresATitle(t *testing.T) {
	require.NoError(t, (&NotifyActionConfig{Title: "hi"}).Validate())
	require.Error(t, (&NotifyActionConfig{}).Validate())
}

// TestNotifyRaiserCoversExactlyNotify guards the declared capability
// notifyNodeConfig relies on instead of a type switch. An interface assertion
// fails closed for any config that does not implement it, which is invisible
// when a config should have the capability and silently does not — so this
// test re-derives flow's currently declared (non-source) node types from
// flow.NodeTypes() minus sources.Types() and requires every one of them be
// accounted for in the sample below. Adding a new terminal node type without
// updating this test trips the count check, forcing a conscious decision
// about whether it raises a notification, rather than letting it silently
// fall through the assertion in notifyNodeConfig.
func TestNotifyRaiserCoversExactlyNotify(t *testing.T) {
	sample := map[string]flow.NodeConfig{
		"feed":           &flow.FeedConfig{},
		"action":         &flow.ActionConfig{},
		"notify":         &flow.NotifyConfig{},
		"function":       &flow.FunctionConfig{},
		"github-filter":  &flow.GithubFilterConfig{},
		"launch-session": &flow.LaunchSessionConfig{},
		"launch-chat":    &flow.LaunchChatConfig{},
	}
	raises := map[string]bool{"notify": true}

	excludedSourceTypes := make(map[string]bool)
	for _, sourceType := range sources.Types() {
		excludedSourceTypes[sourceType] = true
	}
	declaredCount := 0
	for _, nodeType := range flow.NodeTypes() {
		if !excludedSourceTypes[nodeType] {
			declaredCount++
		}
	}
	require.Lenf(t, sample, declaredCount,
		"flow's declared (non-source) node types changed; update the sample map above so this test still covers all of them")

	for nodeType, cfg := range sample {
		_, ok := cfg.(notifyRaiser)
		assert.Equalf(t, raises[nodeType], ok, "node type %q's notifyRaiser membership changed unexpectedly", nodeType)
	}

	// A source connector's config is a distinct wrapper type and never
	// raises a notify output, regardless of which connector it wraps.
	var sourceCfg flow.NodeConfig = &flow.SourceConfig{}
	_, ok := sourceCfg.(notifyRaiser)
	assert.False(t, ok)
}

// A feed never raises a notify output, so a notify id targeting one resolves
// to nothing — exactly like any other node whose config is not a raiser.
func TestFlowActions_AFeedIsNotANotifyTarget(t *testing.T) {
	flows := flowListerTest{flows: []flow.Flow{{ID: "triage", Nodes: []flow.Node{
		{ID: "review-requests", Type: "feed", Name: "Review requests", Config: &flow.FeedConfig{Icon: "eye"}},
	}}}}

	_, ok := NewFlowActions(flows, nil).Get(models.NotifyActionID("triage/review-requests"))
	assert.False(t, ok)
}

func launchFlows() flowListerTest {
	return flowListerTest{flows: []flow.Flow{{
		ID: "triage",
		Nodes: []flow.Node{
			{ID: "review", Type: "launch-session", Name: "Review PRs", Config: &flow.LaunchSessionConfig{
				Repo: "{{ .Payload.repo }}", Agent: "claude", SessionName: "review-{{ .Payload.num }}", Prompt: "Review {{ .Payload.num }}",
			}},
			{ID: "chat", Type: "launch-chat", Config: &flow.LaunchChatConfig{Workspace: "incident-triage", Prompt: "Triage {{ .Payload.title }}"}},
			{ID: "tell-me", Type: "notify", Config: &flow.NotifyConfig{Title: "hi"}},
		},
	}}}
}

func TestFlowActions_ProjectsALaunchSessionNode(t *testing.T) {
	action, ok := NewFlowActions(launchFlows(), nil).Get(models.LaunchActionID("triage/review"))
	require.True(t, ok)
	assert.Equal(t, actions.Action{
		ID: models.LaunchActionID("triage/review"), Label: "Review PRs", Type: ActionTypeLaunchSession,
		Config: &LaunchNodeActionConfig{
			PromptTemplate: "Review {{ .Payload.num }}", RepoTemplate: "{{ .Payload.repo }}", Agent: "claude",
			NameTemplate: "review-{{ .Payload.num }}",
		},
	}, action)
}

func TestFlowActions_ProjectsALaunchChatNode(t *testing.T) {
	action, ok := NewFlowActions(launchFlows(), nil).Get(models.LaunchActionID("triage/chat"))
	require.True(t, ok)
	assert.Equal(t, actions.Action{
		ID: models.LaunchActionID("triage/chat"), Label: "Launch chat chat", Type: ActionTypeLaunchSession,
		Config: &LaunchNodeActionConfig{PromptTemplate: "Triage {{ .Payload.title }}", Workspace: "incident-triage"},
	}, action)
}

// A launch id whose node is gone, or was retyped, fails its queued command
// rather than running something else.
func TestFlowActions_UnknownLaunchTargets(t *testing.T) {
	lister := NewFlowActions(launchFlows(), nil)
	for _, target := range []string{"triage/missing", "other/review", "triage/tell-me", "no-slash"} {
		_, ok := lister.Get(models.LaunchActionID(target))
		assert.Falsef(t, ok, "target %q", target)
	}
	_, ok := lister.Get(models.NotifyActionID("triage/review"))
	assert.False(t, ok, "a launch node is not a notify target")
}
