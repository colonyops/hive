package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
)

type terminalConfirmer interface {
	ConfirmTerminal(ctx context.Context, refs []feed.AbsentRef) ([]ghclient.ItemState, error)
}

type absenceConfirmer struct{ live terminalConfirmer }

// resolvedAbsence pairs a prior observation with its decoded feed item, kept
// index-parallel to the refs sent to ConfirmTerminal.
type resolvedAbsence struct {
	observation models.Observation
	item        feed.Item
}

func (c *absenceConfirmer) ConfirmAbsence(ctx context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	resolvable := make([]resolvedAbsence, 0, len(previous))
	refs := make([]feed.AbsentRef, 0, len(previous))
	for _, observation := range previous {
		var item feed.Item
		if err := json.Unmarshal(observation.Payload, &item); err != nil {
			continue
		}
		owner, name, ok := strings.Cut(item.Repo, "/")
		if item.Num <= 0 || !ok || owner == "" || name == "" {
			continue
		}
		resolvable = append(resolvable, resolvedAbsence{observation: observation, item: item})
		refs = append(refs, feed.AbsentRef{Repo: item.Repo, Num: item.Num})
	}

	states, err := c.live.ConfirmTerminal(ctx, refs)

	verdicts := make(map[string]models.AbsenceVerdict)
	for i, st := range states {
		if !st.Found {
			continue
		}
		observation, item := resolvable[i].observation, resolvable[i].item
		item.State = st.State
		item.UpdatedAt = st.UpdatedAt.UnixMilli()
		payload, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			continue
		}
		current := observation
		// source_head stores only payload. Set display metadata from its decoded
		// feed item so absence hydration never overwrites the inbox with blanks.
		current.Title, current.URL = item.Title, item.URL
		current.Payload, current.ObservedAt = payload, item.UpdatedAt
		verdicts[observation.ExternalID] = models.AbsenceVerdict{Current: &current, Terminal: terminalState(st.State)}
	}
	return verdicts, err
}

func terminalState(state string) bool {
	return state == "closed" || state == "merged"
}

type classifier struct{}

func (classifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	cur := decodeGithub(current.Payload)
	lifecycle := models.LifecycleUnknown
	if cur.State == "open" {
		lifecycle = models.LifecycleActive
	}
	if terminalState(cur.State) {
		lifecycle = models.LifecycleTerminal
	}
	out := models.Classification{Kind: "updated", Transition: models.TransitionNone, Attention: models.AttentionTrivial, Lifecycle: lifecycle, SourceState: cur.State}
	if previous == nil {
		out.Attention, out.Kind, out.Summary, out.OccurrenceKey = models.AttentionActivity, "observed", "Added to workspace", githubOccurrence(current.ExternalID, cur)
		out.Detail = githubDetail(nil, cur)
		return out
	}
	prev := decodeGithub(previous.Payload)
	prevTerminal := terminalState(prev.State)
	curTerminal := terminalState(cur.State)
	switch {
	case !prevTerminal && curTerminal:
		out.Kind, out.Summary, out.Transition, out.Attention, out.ArchivedReason = cur.State, titleCase(cur.State), models.TransitionEnteredTerminal, models.AttentionActivity, cur.State
	case prevTerminal && !curTerminal && cur.State == "open":
		out.Kind, out.Summary, out.Transition, out.Attention = "reopened", "Reopened", models.TransitionLeftTerminal, models.AttentionActivity
	case cur.UpdatedAt > prev.UpdatedAt && cur.Reason != "":
		// Notification payloads do not carry labels, while search payloads do.
		// Prefer GitHub's explicit notification reason before comparing labels
		// so a comment cannot look like every label was removed.
		out.Kind, out.Summary, out.Attention = githubActivity(cur.Reason)
	case !sameLabels(cur.Labels, prev.Labels):
		out.Kind, out.Summary, out.Attention = "labels", labelChangeSummary(prev.Labels, cur.Labels), models.AttentionActivity
	case cur.UpdatedAt > prev.UpdatedAt:
		out.Kind, out.Summary, out.Attention = githubActivity(cur.Reason)
	}
	if out.Attention == models.AttentionActivity || out.Transition != models.TransitionNone {
		out.OccurrenceKey = githubOccurrence(current.ExternalID, cur)
		out.Detail = githubDetail(&prev, cur)
	}
	return out
}

type githubPayload struct {
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
	UpdatedAt int64    `json:"updatedAt"`
	Labels    []string `json:"labels"`
}

func decodeGithub(payload []byte) githubPayload {
	var p githubPayload
	_ = json.Unmarshal(payload, &p)
	p.State = strings.ToLower(p.State)
	return p
}

func githubOccurrence(id string, p githubPayload) string {
	return fmt.Sprintf("%s:%s:%d:%s", id, p.State, p.UpdatedAt, p.Reason)
}

func githubDetail(previous *githubPayload, current githubPayload) []byte {
	detail := map[string]any{
		"state": current.State, "reason": current.Reason,
		"updatedAt": current.UpdatedAt, "labels": current.Labels,
	}
	if previous != nil {
		detail["previousState"] = previous.State
		detail["previousLabels"] = previous.Labels
	}
	b, _ := json.Marshal(detail)
	return b
}

func githubActivity(reason string) (kind, summary string, attention models.Attention) {
	summaries := map[string]string{
		"approval_requested": "Approval requested",
		"assign":             "Assigned on GitHub",
		"author":             "New activity from the author",
		"ci_activity":        "CI status changed",
		"comment":            "New comment activity",
		"mention":            "Mentioned on GitHub",
		"review_requested":   "Review requested",
		"state_change":       "State changed on GitHub",
		"subscribed":         "New subscribed activity",
		"team_mention":       "Team mentioned on GitHub",
	}
	if summary = summaries[reason]; summary == "" {
		summary = "Updated on GitHub"
	}
	kind = reason
	if kind == "" {
		kind = "updated"
	}
	return kind, summary, models.AttentionActivity
}

func labelChangeSummary(previous, current []string) string {
	before := make(map[string]bool, len(previous))
	after := make(map[string]bool, len(current))
	for _, label := range previous {
		before[label] = true
	}
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

func titleCase(value string) string {
	if value == "" {
		return "State changed"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func sameLabels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
		if m[v] < 0 {
			return false
		}
	}
	return true
}
