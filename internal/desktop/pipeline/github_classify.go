package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

type githubAbsenceConfirmer struct{ live *feed.LiveProvider }

func (c *githubAbsenceConfirmer) ConfirmAbsence(ctx context.Context, prev Observation) (AbsenceVerdict, error) {
	var item feed.Item
	if err := json.Unmarshal(prev.Payload, &item); err != nil {
		return AbsenceVerdict{}, fmt.Errorf("decoding GitHub observation: %w", err)
	}
	issue, err := c.live.ConfirmTerminal(ctx, item.Repo, item.Num, item.Kind == "PR")
	if err != nil {
		return AbsenceVerdict{}, err
	}
	state := issue.State
	if issue.Merged {
		state = "merged"
	}
	item.State = state
	item.UpdatedAt = issue.UpdatedAt.UnixMilli()
	payload, err := json.Marshal(item)
	if err != nil {
		return AbsenceVerdict{}, err
	}
	current := prev
	// source_head stores only payload. Set display metadata from its decoded
	// feed item so absence hydration never overwrites the inbox with blanks.
	current.Title, current.URL = item.Title, item.URL
	current.Payload, current.ObservedAt = payload, item.UpdatedAt
	return AbsenceVerdict{Current: &current, Terminal: state == "closed" || state == "merged"}, nil
}

type githubClassifier struct{ absence AbsenceConfirmer }

func newGithubClassifier(absence AbsenceConfirmer) *githubClassifier {
	return &githubClassifier{absence: absence}
}

func (c *githubClassifier) ConfirmAbsence(ctx context.Context, prev Observation) (AbsenceVerdict, error) {
	return c.absence.ConfirmAbsence(ctx, prev)
}

func (c *githubClassifier) Classify(previous *Observation, current Observation) Classification {
	cur := decodeGithub(current.Payload)
	lifecycle := pipelinedb.LifecycleUnknown
	if cur.State == "open" {
		lifecycle = pipelinedb.LifecycleActive
	}
	if cur.State == "closed" || cur.State == "merged" {
		lifecycle = pipelinedb.LifecycleTerminal
	}
	out := Classification{Kind: "updated", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionTrivial, Lifecycle: lifecycle, SourceState: cur.State}
	if previous == nil {
		out.Attention, out.Kind, out.Summary, out.OccurrenceKey = pipelinedb.AttentionActivity, "observed", "Added to workspace", githubOccurrence(current.ExternalID, cur)
		out.Detail = githubDetail(nil, cur)
		return out
	}
	prev := decodeGithub(previous.Payload)
	prevTerminal := prev.State == "closed" || prev.State == "merged"
	curTerminal := cur.State == "closed" || cur.State == "merged"
	switch {
	case !prevTerminal && curTerminal:
		out.Kind, out.Summary, out.Transition, out.Attention, out.ArchivedReason = cur.State, titleCase(cur.State), pipelinedb.TransitionEnteredTerminal, pipelinedb.AttentionActivity, cur.State
	case prevTerminal && !curTerminal && cur.State == "open":
		out.Kind, out.Summary, out.Transition, out.Attention = "reopened", "Reopened", pipelinedb.TransitionLeftTerminal, pipelinedb.AttentionActivity
	case cur.UpdatedAt > prev.UpdatedAt && cur.Reason != "":
		// Notification payloads do not carry labels, while search payloads do.
		// Prefer GitHub's explicit notification reason before comparing labels
		// so a comment cannot look like every label was removed.
		out.Kind, out.Summary, out.Attention = githubActivity(cur.Reason)
	case !sameLabels(cur.Labels, prev.Labels):
		out.Kind, out.Summary, out.Attention = "labels", labelChangeSummary(prev.Labels, cur.Labels), pipelinedb.AttentionActivity
	case cur.UpdatedAt > prev.UpdatedAt:
		out.Kind, out.Summary, out.Attention = githubActivity(cur.Reason)
	}
	if out.Attention == pipelinedb.AttentionActivity || out.Transition != pipelinedb.TransitionNone {
		out.OccurrenceKey = githubOccurrence(current.ExternalID, cur)
		out.Detail = githubDetail(&prev, cur)
	}
	return out
}

func NewGithubSourceAdapter(live *feed.LiveProvider) SourceAdapter {
	return newGithubSourceAdapter(live)
}

func newGithubSourceAdapter(live *feed.LiveProvider) SourceAdapter {
	absence := &githubAbsenceConfirmer{live: live}
	classifier := newGithubClassifier(absence)
	return SourceAdapter{SourceKind: "github", Classifier: classifier, AbsenceConfirmer: classifier}
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

func githubActivity(reason string) (kind, summary string, attention pipelinedb.Attention) {
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
	return kind, summary, pipelinedb.AttentionActivity
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
