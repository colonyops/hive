package main

import (
	"fmt"
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/internal/web/extractors"
)

// The vocabulary is deliberately GitHub's own, not an invented one. The
// desktop's sources.github reads exactly four things — state, updatedAt,
// labels, and a notification reason — so those are the only levers that exist.
// In particular there is no "approved" action: GitHub has no such notification
// reason, and the desktop never fetches review state. An approval reaches the
// app as activity on the item, which is what these reasons produce.

type quickAction struct {
	label string
	apply func() Mutations
}

var quickActions = map[string]quickAction{
	"review-requested":   {"Review requested", func() Mutations { return Mutations{Reason: new("review_requested")} }},
	"approval-requested": {"Approval requested", func() Mutations { return Mutations{Reason: new("approval_requested")} }},
	"comment":            {"New comment", func() Mutations { return Mutations{Reason: new("comment")} }},
	"ci-activity":        {"CI status changed", func() Mutations { return Mutations{Reason: new("ci_activity")} }},
	"mention":            {"Mentioned", func() Mutations { return Mutations{Reason: new("mention")} }},
	"state-change":       {"State changed", func() Mutations { return Mutations{Reason: new("state_change")} }},
	// Terminal transitions also mark the item absent from search results,
	// because that is how GitHub behaves once it leaves an is:open query — and
	// it is the only way to exercise the desktop's ConfirmAbsence path.
	"merge":  {"Merge", func() Mutations { return Mutations{State: new("merged"), Absent: new(true)} }},
	"close":  {"Close", func() Mutations { return Mutations{State: new("closed"), Absent: new(true)} }},
	"reopen": {"Reopen", func() Mutations { return Mutations{State: new("open"), Absent: new(false)} }},
	"draft":  {"Mark draft", func() Mutations { return Mutations{Draft: new(true)} }},
	"ready":  {"Mark ready", func() Mutations { return Mutations{Draft: new(false)} }},
}

// actionOrder fixes the dashboard's button order; map iteration would shuffle
// them on every poll.
var actionOrder = []string{
	"review-requested", "approval-requested", "comment", "ci-activity", "mention",
	"state-change", "draft", "ready", "reopen", "close", "merge",
}

type actionRequest struct {
	Repo   string `json:"repo"`
	Num    int    `json:"num"`
	Action string `json:"action"`
}

func (r actionRequest) Validate() error {
	return criterio.ValidateStruct(
		matcherErrors(Matcher{Repo: r.Repo, Num: r.Num}),
		criterio.Run("action", r.Action, knownAction),
	)
}

func knownAction(val string) error {
	if _, ok := quickActions[val]; !ok {
		return fmt.Errorf("unknown action %q", val)
	}
	return nil
}

func (c *Control) Action(w http.ResponseWriter, r *http.Request) error {
	req, err := extractors.Body[actionRequest](w, r)
	if err != nil {
		return err
	}
	match := Matcher{Repo: req.Repo, Num: req.Num}
	merged := c.store.Apply(match.Key(), quickActions[req.Action].apply())
	c.logger.Info().Str("item", match.Key()).Str("action", req.Action).Msg("action applied")
	return server.JSON(w, http.StatusOK, overlayResponse{Item: match.Key(), Overlay: merged})
}
