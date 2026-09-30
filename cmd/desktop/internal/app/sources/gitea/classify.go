package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// terminalState reports whether a state ends an item's lifecycle. Gitea reports
// a merged pull request as closed and records the merge separately; the fetch
// layer folds that into "merged", so both words appear here.
func terminalState(state string) bool {
	return state == "closed" || state == "merged"
}

// classifier turns a Gitea observation into an inbox event.
//
// It is not the canonical classifier: a Gitea payload carries the forge's own
// open/closed/merged vocabulary rather than the canonical `resolved`/`done`
// one, and "merged" is a distinct outcome the canonical contract has no word
// for.
type classifier struct{}

var _ models.Classifier = classifier{}

func (classifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	cur := decodePayload(current.Payload)

	lifecycle := models.LifecycleUnknown
	switch {
	case cur.State == "open":
		lifecycle = models.LifecycleActive
	case terminalState(cur.State):
		lifecycle = models.LifecycleTerminal
	}

	out := models.Classification{
		Kind:        "updated",
		Transition:  models.TransitionNone,
		Attention:   models.AttentionTrivial,
		Lifecycle:   lifecycle,
		SourceState: cur.State,
	}
	if previous == nil {
		out.Attention, out.Kind, out.Summary = models.AttentionActivity, "observed", "Added to workspace"
		out.OccurrenceKey, out.Detail = occurrenceKey(current.ExternalID, cur), detail(nil, cur)
		return out
	}

	prev := decodePayload(previous.Payload)
	prevTerminal, curTerminal := terminalState(prev.State), terminalState(cur.State)
	switch {
	case !prevTerminal && curTerminal:
		out.Kind, out.Summary = cur.State, titleCase(cur.State)
		out.Transition, out.Attention, out.ArchivedReason = models.TransitionEnteredTerminal, models.AttentionActivity, cur.State
	case prevTerminal && !curTerminal && cur.State == "open":
		out.Kind, out.Summary = "reopened", "Reopened"
		out.Transition, out.Attention = models.TransitionLeftTerminal, models.AttentionActivity
	case comparableLabels(prev, cur) && !sameLabels(prev.Labels, cur.Labels):
		out.Kind, out.Summary, out.Attention = "labels", labelChangeSummary(prev.Labels, cur.Labels), models.AttentionActivity
	case cur.UpdatedAt > prev.UpdatedAt:
		out.Kind, out.Summary, out.Attention = "updated", "Updated on Gitea", models.AttentionActivity
	}
	if out.Attention == models.AttentionActivity || out.Transition != models.TransitionNone {
		out.OccurrenceKey, out.Detail = occurrenceKey(current.ExternalID, cur), detail(&prev, cur)
	}
	return out
}

// payload is the subset of an Item the classifier reads.
type payload struct {
	State     string   `json:"state"`
	Origin    string   `json:"origin"`
	UpdatedAt int64    `json:"updatedAt"`
	Labels    []string `json:"labels"`
}

func decodePayload(raw []byte) payload {
	var p payload
	_ = json.Unmarshal(raw, &p)
	p.State = strings.ToLower(p.State)
	return p
}

// comparableLabels reports whether two observations' label sets mean the same
// thing. A notification carries no labels at all, so an item seen through both
// a notifications source and a search source alternates between a full label
// set and an empty one — and comparing across that would report every label
// removed, then re-added, on every tick. GitHub defends against this with its
// notification reason; Gitea sends none, so the payload records where it came
// from instead.
func comparableLabels(previous, current payload) bool {
	return previous.Origin == current.Origin
}

func occurrenceKey(id string, p payload) string {
	return fmt.Sprintf("%s:%s:%d", id, p.State, p.UpdatedAt)
}

func detail(previous *payload, current payload) []byte {
	out := map[string]any{
		"state":     current.State,
		"updatedAt": current.UpdatedAt,
		"labels":    current.Labels,
		"origin":    current.Origin,
	}
	if previous != nil {
		out["previousState"] = previous.State
		out["previousLabels"] = previous.Labels
	}
	encoded, _ := json.Marshal(out)
	return encoded
}

func labelChangeSummary(previous, current []string) string {
	before := make(map[string]bool, len(previous))
	for _, label := range previous {
		before[label] = true
	}
	after := make(map[string]bool, len(current))
	for _, label := range current {
		after[label] = true
	}

	added, removed := make([]string, 0), make([]string, 0)
	for _, label := range current {
		if !before[label] {
			added = append(added, label)
		}
	}
	for _, label := range previous {
		if !after[label] {
			removed = append(removed, label)
		}
	}
	switch {
	case len(added) > 0 && len(removed) == 0:
		return "Labels added: " + strings.Join(added, ", ")
	case len(removed) > 0 && len(added) == 0:
		return "Labels removed: " + strings.Join(removed, ", ")
	default:
		return "Labels changed"
	}
}

func sameLabels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, label := range a {
		counts[label]++
	}
	for _, label := range b {
		counts[label]--
		if counts[label] < 0 {
			return false
		}
	}
	return true
}

func titleCase(value string) string {
	if value == "" {
		return "State changed"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

// itemStates is the fetch half of absence confirmation, a field so a test can
// answer without the network.
type itemStates interface {
	ItemStates(ctx context.Context, refs []itemRef) ([]itemState, error)
}

type absenceConfirmer struct{ fetcher itemStates }

var _ models.AbsenceConfirmer = (*absenceConfirmer)(nil)

// resolvedAbsence pairs a prior observation with its decoded item, kept
// index-parallel to the refs sent to ItemStates.
type resolvedAbsence struct {
	observation models.Observation
	item        Item
}

// ConfirmAbsence hydrates the current state of items that left a source's
// result set, so the producer can tell a merge or a close from an item that
// merely aged out of the page or stopped matching the filters.
//
// An item that no longer resolves — deleted, or in a repository the token lost
// access to — returns no verdict, leaving it to the flow's resurface policy
// rather than archiving it on a lookup failure.
func (c *absenceConfirmer) ConfirmAbsence(ctx context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	resolvable := make([]resolvedAbsence, 0, len(previous))
	refs := make([]itemRef, 0, len(previous))
	for _, observation := range previous {
		var item Item
		if err := json.Unmarshal(observation.Payload, &item); err != nil {
			continue
		}
		owner, name, ok := strings.Cut(item.Repo, "/")
		if item.Num <= 0 || !ok || owner == "" || name == "" {
			continue
		}
		resolvable = append(resolvable, resolvedAbsence{observation: observation, item: item})
		refs = append(refs, itemRef{Repo: item.Repo, Num: item.Num})
	}

	states, err := c.fetcher.ItemStates(ctx, refs)

	verdicts := make(map[string]models.AbsenceVerdict)
	for i, state := range states {
		if !state.Found {
			continue
		}
		observation, item := resolvable[i].observation, resolvable[i].item
		item.State, item.UpdatedAt = state.State, state.UpdatedAt
		if state.Title != "" {
			item.Title = state.Title
		}
		if state.URL != "" {
			item.URL = state.URL
		}
		payload, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			continue
		}
		current := observation
		// source_head stores only the payload, so display metadata is set from
		// the decoded item — absence hydration must never blank the inbox row.
		current.Title, current.URL = item.Title, item.URL
		current.Payload, current.ObservedAt = payload, item.UpdatedAt
		verdicts[observation.ExternalID] = models.AbsenceVerdict{Current: &current, Terminal: terminalState(state.State)}
	}
	return verdicts, err
}
