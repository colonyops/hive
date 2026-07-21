package main

import (
	"fmt"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
)

// FlowSummary is one flow file's listing row for the flows picker: identity
// plus load/validity status, so a broken flow file shows up (with its
// error) instead of silently vanishing from the list.
type FlowSummary struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Valid    bool     `json:"valid"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// FlowsService is the Wails service exposing the desktop pipeline's flow
// definitions to the frontend graph editor. All data comes from the
// injected *flow.FlowStore; this type is wire glue only, matching
// FeedService/PipelineService's thin-glue style.
type FlowsService struct {
	store *flow.FlowStore
}

func NewFlowsService(store *flow.FlowStore) *FlowsService {
	return &FlowsService{store: store}
}

// ListFlows returns one summary per flow file — valid and invalid alike —
// for the flows picker.
func (s *FlowsService) ListFlows() ([]FlowSummary, error) {
	statuses := s.store.Statuses()
	out := make([]FlowSummary, 0, len(statuses))
	for _, st := range statuses {
		summary := FlowSummary{ID: st.ID, Valid: st.Valid, Warnings: st.Warnings}
		if st.Valid {
			summary.Name = st.Flow.Name
			summary.Enabled = st.Flow.Enabled
		} else if st.Err != nil {
			summary.Error = st.Err.Error()
		}
		out = append(out, summary)
	}
	return out, nil
}

// CreateFlow seeds a new flow (a new "profile") named name and returns its
// listing summary, so the frontend can select it immediately.
func (s *FlowsService) CreateFlow(name string) (FlowSummary, error) {
	f, err := s.store.Create(name)
	if err != nil {
		return FlowSummary{}, err
	}
	return FlowSummary{ID: f.ID, Name: f.Name, Enabled: f.Enabled, Valid: true}, nil
}

// RenameFlow changes a flow's profile-facing display name while preserving its
// stable id and graph definition.
func (s *FlowsService) RenameFlow(id, name string) (FlowSummary, error) {
	f, err := s.store.Rename(id, name)
	if err != nil {
		return FlowSummary{}, err
	}
	return FlowSummary{ID: f.ID, Name: f.Name, Enabled: f.Enabled, Valid: true}, nil
}

// DeleteFlow removes a flow (a "profile") and its layout.
func (s *FlowsService) DeleteFlow(id string) error {
	return s.store.Delete(id)
}

// GetFlow returns one flow's full definition for the editor.
func (s *FlowsService) GetFlow(id string) (flow.Flow, error) {
	f, ok := s.store.Get(id)
	if !ok {
		return flow.Flow{}, fmt.Errorf("flow %q not found", id)
	}
	return f, nil
}

// SaveFlow validates and persists a flow's definition. An invalid flow is
// rejected and the last-good file on disk — and the store's served flow —
// are left untouched.
func (s *FlowsService) SaveFlow(f flow.Flow) error {
	return s.store.Save(f)
}

// GetLayout returns a flow's node layout (canvas positions). A missing or
// broken layout file is not an error: it returns an empty Layout so the
// editor lays out nodes fresh.
func (s *FlowsService) GetLayout(id string) flow.Layout {
	return s.store.GetLayout(id)
}

// SaveLayout persists a flow's node layout.
func (s *FlowsService) SaveLayout(id string, layout flow.Layout) error {
	return s.store.SaveLayout(id, layout)
}

// GetSidebar returns a flow's sidebar layout: how its feed nodes are grouped
// into folders and ordered in the sidebar's FEEDS section. A missing or broken
// file is not an error — it returns an empty layout so the sidebar falls back
// to flow-node order.
func (s *FlowsService) GetSidebar(id string) flow.SidebarLayout {
	return s.store.GetSidebar(id)
}

// SaveSidebar persists a flow's sidebar layout (feed folders + ordering).
func (s *FlowsService) SaveSidebar(id string, layout flow.SidebarLayout) error {
	return s.store.SaveSidebar(id, layout)
}
