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
	out := Classification{Kind: "updated", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionTrivial, Lifecycle: lifecycle, SourceState: cur.State, Summary: current.Title, Detail: githubDetail(cur)}
	if previous == nil {
		out.Attention, out.Kind, out.OccurrenceKey = pipelinedb.AttentionActivity, "observed", githubOccurrence(current.ExternalID, cur)
		return out
	}
	prev := decodeGithub(previous.Payload)
	prevTerminal := prev.State == "closed" || prev.State == "merged"
	curTerminal := cur.State == "closed" || cur.State == "merged"
	switch {
	case !prevTerminal && curTerminal:
		out.Kind, out.Transition, out.Attention, out.ArchivedReason = cur.State, pipelinedb.TransitionEnteredTerminal, pipelinedb.AttentionActivity, cur.State
	case prevTerminal && !curTerminal && cur.State == "open":
		out.Kind, out.Transition, out.Attention = "reopened", pipelinedb.TransitionLeftTerminal, pipelinedb.AttentionActivity
	case cur.Reason == "review_requested" || cur.UpdatedAt > prev.UpdatedAt || !sameLabels(cur.Labels, prev.Labels):
		out.Kind, out.Attention = "activity", pipelinedb.AttentionActivity
	}
	if out.Attention == pipelinedb.AttentionActivity || out.Transition != pipelinedb.TransitionNone {
		out.OccurrenceKey = githubOccurrence(current.ExternalID, cur)
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

func githubDetail(p githubPayload) []byte {
	b, _ := json.Marshal(struct {
		State, Reason string
		UpdatedAt     int64
	}{p.State, p.Reason, p.UpdatedAt})
	return b
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
