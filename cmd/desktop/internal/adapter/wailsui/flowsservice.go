package wailsui

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// FlowSummary is one flow file's listing row: identity plus load status, so a
// broken flow file shows up with its error instead of silently vanishing.
type FlowSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Valid   bool   `json:"valid"`
	// Image is the profile's avatar as a data URL, or empty when it has none
	// (the rail falls back to the letter chip). Encoding the small stored PNG
	// inline keeps the rail a pure prop render with no second fetch.
	Image string `json:"image,omitempty"`
	// Nodes counts the flow's nodes; the starter graph is seeded only into a
	// profile that has none.
	Nodes    int      `json:"nodes"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// FlowsService exposes the flow definitions to the frontend graph editor.
type FlowsService struct {
	flows *app.FlowsService
}

func NewFlowsService(flows *app.FlowsService) *FlowsService {
	return &FlowsService{flows: flows}
}

// ListFlows returns one summary per flow file, valid and invalid alike.
func (s *FlowsService) ListFlows(ctx context.Context) ([]FlowSummary, error) {
	statuses := s.flows.Statuses(ctx)
	out := make([]FlowSummary, 0, len(statuses))
	for _, st := range statuses {
		summary := FlowSummary{ID: st.ID, Valid: st.Valid, Warnings: st.Warnings}
		if st.Valid {
			summary.Name = st.Flow.Name
			summary.Enabled = st.Flow.Enabled
			summary.Nodes = len(st.Flow.Nodes)
			if st.Flow.Image != "" {
				summary.Image = s.imageDataURL(ctx, st.ID)
			}
		} else if st.Err != nil {
			summary.Error = st.Err.Error()
		}
		out = append(out, summary)
	}
	return out, nil
}

func (s *FlowsService) CreateFlow(ctx context.Context, name string) (FlowSummary, error) {
	return summarize(s.flows.Create(ctx, name))
}

// SeedStarterFlow fills an empty profile with the starter graph, fetching as
// the one connected GitHub account.
func (s *FlowsService) SeedStarterFlow(ctx context.Context, id string) (FlowSummary, error) {
	return summarize(s.flows.SeedStarter(ctx, id))
}

func (s *FlowsService) RenameFlow(ctx context.Context, id, name string) (FlowSummary, error) {
	return summarize(s.flows.Rename(ctx, id, name))
}

func (s *FlowsService) SetFlowEnabled(ctx context.Context, id string, enabled bool) (FlowSummary, error) {
	return summarize(s.flows.SetEnabled(ctx, id, enabled))
}

// SetFlowOrder persists the rail order — ids top first, the whole rail in one
// write.
func (s *FlowsService) SetFlowOrder(ctx context.Context, ids []string) error {
	return s.flows.SetOrder(ctx, ids)
}

func (s *FlowsService) DeleteFlow(ctx context.Context, id string) error {
	return s.flows.Delete(ctx, id)
}

func (s *FlowsService) GetFlow(ctx context.Context, id string) (flow.Flow, error) {
	return s.flows.Get(ctx, id)
}

func (s *FlowsService) SaveFlow(ctx context.Context, f flow.Flow) error {
	return s.flows.Save(ctx, f)
}

// SetProfileImage sets a profile's sidebar-rail avatar. The frontend sends the
// picked file as base64 (a bare payload or a data: URL); the core normalizes,
// stores, and references it. The returned summary carries the stored image so
// the settings view and rail can preview it without a re-fetch.
func (s *FlowsService) SetProfileImage(ctx context.Context, id, data string) (FlowSummary, error) {
	raw, err := decodeImagePayload(data)
	if err != nil {
		return FlowSummary{}, err
	}
	f, err := s.flows.SetProfileImage(ctx, id, raw)
	if err != nil {
		return FlowSummary{}, err
	}
	return s.summaryWithImage(ctx, f), nil
}

// ClearProfileImage removes a profile's avatar, returning the summary with no
// image so the rail reverts to the letter chip.
func (s *FlowsService) ClearProfileImage(ctx context.Context, id string) (FlowSummary, error) {
	f, err := s.flows.ClearProfileImage(ctx, id)
	if err != nil {
		return FlowSummary{}, err
	}
	return s.summaryWithImage(ctx, f), nil
}

// MarkImageView is a stored feed-mark image: the content Hash a source node
// records in its config and the normalized PNG as a data URL for preview.
type MarkImageView struct {
	Hash  string `json:"hash"`
	Image string `json:"image"`
}

// SetMarkImage stores an uploaded feed-mark image (base64, bare or a data: URL)
// and returns its hash and stored PNG for preview. The hash reaches the flow
// through the node editor's ordinary graph save.
func (s *FlowsService) SetMarkImage(ctx context.Context, data string) (MarkImageView, error) {
	raw, err := decodeImagePayload(data)
	if err != nil {
		return MarkImageView{}, err
	}
	hash, err := s.flows.StoreMarkImage(ctx, raw)
	if err != nil {
		return MarkImageView{}, err
	}
	return MarkImageView{Hash: hash, Image: s.markDataURL(ctx, hash)}, nil
}

// MarkImages resolves feed-mark hashes to PNG data URLs. A hash with no stored
// file is omitted, so the feed falls back to the glyph.
func (s *FlowsService) MarkImages(ctx context.Context, hashes []string) (map[string]string, error) {
	out := make(map[string]string, len(hashes))
	for _, hash := range hashes {
		if _, done := out[hash]; done {
			continue
		}
		if url := s.markDataURL(ctx, hash); url != "" {
			out[hash] = url
		}
	}
	return out, nil
}

// markDataURL reads a stored mark PNG as a data URL, or "" when the hash
// resolves to no file.
func (s *FlowsService) markDataURL(ctx context.Context, hash string) string {
	data, ok, err := s.flows.MarkImage(ctx, hash)
	if err != nil || !ok || len(data) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func (s *FlowsService) GetLayout(ctx context.Context, id string) flow.Layout {
	return s.flows.Layout(ctx, id)
}

func (s *FlowsService) SaveLayout(ctx context.Context, id string, layout flow.Layout) error {
	return s.flows.SaveLayout(ctx, id, layout)
}

func (s *FlowsService) GetSidebar(ctx context.Context, id string) flow.SidebarLayout {
	return s.flows.Sidebar(ctx, id)
}

func (s *FlowsService) SaveSidebar(ctx context.Context, id string, layout flow.SidebarLayout) error {
	return s.flows.SaveSidebar(ctx, id, layout)
}

// summarize projects a freshly written flow onto the listing row the editor
// selects with. A flow that just round-tripped through Save is valid by
// construction.
func summarize(f flow.Flow, err error) (FlowSummary, error) {
	if err != nil {
		return FlowSummary{}, err
	}
	return FlowSummary{ID: f.ID, Name: f.Name, Enabled: f.Enabled, Valid: true}, nil
}

// summaryWithImage is summarize for an image mutation: the flow is valid by
// construction and its avatar (if any) is inlined as a data URL.
func (s *FlowsService) summaryWithImage(ctx context.Context, f flow.Flow) FlowSummary {
	summary := FlowSummary{ID: f.ID, Name: f.Name, Enabled: f.Enabled, Valid: true}
	if f.Image != "" {
		summary.Image = s.imageDataURL(ctx, f.ID)
	}
	return summary
}

// imageDataURL reads a profile's stored avatar and encodes it as a PNG data
// URL, or "" when the file is missing — a hash referenced with no file on disk
// (a config synced to a fresh machine) reads as no image, not an error.
func (s *FlowsService) imageDataURL(ctx context.Context, id string) string {
	data, err := s.flows.ProfileImage(ctx, id)
	if err != nil || len(data) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

// decodeImagePayload accepts either a bare base64 string or a data: URL and
// returns the raw bytes.
func decodeImagePayload(payload string) ([]byte, error) {
	if strings.HasPrefix(payload, "data:") {
		if i := strings.IndexByte(payload, ','); i >= 0 {
			payload = payload[i+1:]
		}
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return nil, app.Errorf(app.KindInvalid, "That image couldn't be read.")
	}
	return raw, nil
}
