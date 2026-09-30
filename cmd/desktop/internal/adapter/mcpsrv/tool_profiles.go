package mcpsrv

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// profileView is a profile (flow) as this adapter reports it: identity plus
// load status and whether an avatar is set. Deliberately smaller than the Wails
// FlowSummary — no inline image bytes, since get_profile_image answers those.
type profileView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Valid    bool   `json:"valid"`
	HasImage bool   `json:"hasImage"`
}

type profilesResult struct {
	Profiles []profileView `json:"profiles"`
}

func profileViewOf(id, name string, enabled, valid, hasImage bool) profileView {
	return profileView{ID: id, Name: name, Enabled: enabled, Valid: valid, HasImage: hasImage}
}

func (ctrl *Controller) ListProfiles(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, profilesResult, error) {
	statuses := ctrl.core.Flows.Statuses(ctx)
	out := make([]profileView, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, profileViewOf(st.ID, st.Flow.Name, st.Flow.Enabled, st.Valid, st.Flow.Image != ""))
	}
	return nil, profilesResult{Profiles: out}, nil
}

type createProfileInput struct {
	Name string `json:"name" jsonschema:"Display name for the new profile."`
}

func (ctrl *Controller) CreateProfile(ctx context.Context, _ *mcp.CallToolRequest, in createProfileInput) (*mcp.CallToolResult, profileView, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, profileView{}, ctrl.toolError(app.Errorf(app.KindInvalid, "name is required"))
	}
	f, err := ctrl.core.Flows.Create(ctx, in.Name)
	if err != nil {
		return nil, profileView{}, ctrl.toolError(err)
	}
	return nil, profileViewOf(f.ID, f.Name, f.Enabled, true, f.Image != ""), nil
}

type profileInput struct {
	ProfileID string `json:"profileId" jsonschema:"Profile id from list_profiles."`
}

type deleteProfileResult struct {
	Deleted bool `json:"deleted"`
}

func (ctrl *Controller) DeleteProfile(ctx context.Context, _ *mcp.CallToolRequest, in profileInput) (*mcp.CallToolResult, deleteProfileResult, error) {
	if err := ctrl.core.Flows.Delete(ctx, in.ProfileID); err != nil {
		return nil, deleteProfileResult{}, ctrl.toolError(err)
	}
	return nil, deleteProfileResult{Deleted: true}, nil
}

type setProfileImageInput struct {
	ProfileID   string `json:"profileId"   jsonschema:"Profile id from list_profiles."`
	ImageBase64 string `json:"imageBase64" jsonschema:"The image as base64 (PNG, JPEG, GIF or WebP). A data: URL is accepted and its prefix ignored."`
}

func (ctrl *Controller) SetProfileImage(ctx context.Context, _ *mcp.CallToolRequest, in setProfileImageInput) (*mcp.CallToolResult, profileView, error) {
	raw, err := decodeImage(in.ImageBase64)
	if err != nil {
		return nil, profileView{}, ctrl.toolError(err)
	}
	f, err := ctrl.core.Flows.SetProfileImage(ctx, in.ProfileID, raw)
	if err != nil {
		return nil, profileView{}, ctrl.toolError(err)
	}
	return nil, profileViewOf(f.ID, f.Name, f.Enabled, true, f.Image != ""), nil
}

func (ctrl *Controller) ClearProfileImage(ctx context.Context, _ *mcp.CallToolRequest, in profileInput) (*mcp.CallToolResult, profileView, error) {
	f, err := ctrl.core.Flows.ClearProfileImage(ctx, in.ProfileID)
	if err != nil {
		return nil, profileView{}, ctrl.toolError(err)
	}
	return nil, profileViewOf(f.ID, f.Name, f.Enabled, true, f.Image != ""), nil
}

// GetProfileImage answers with an image content block rather than a structured
// value, so a model that can see images gets the avatar itself instead of a
// base64 blob it has to describe.
//
// Both failures are not_found and they must not read alike: "has no image"
// asserts the profile exists, so a mistyped id has to be answered as a
// mistyped id rather than as an avatar-less profile.
func (ctrl *Controller) GetProfileImage(ctx context.Context, _ *mcp.CallToolRequest, in profileInput) (*mcp.CallToolResult, any, error) {
	if _, err := ctrl.core.Flows.Get(ctx, in.ProfileID); err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	data, err := ctrl.core.Flows.ProfileImage(ctx, in.ProfileID)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	if len(data) == 0 {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindNotFound, "profile %q has no image", in.ProfileID))
	}
	return pngResult(data), nil, nil
}

// pngResult wraps stored PNG bytes as the tool's whole answer. The SDK
// base64-encodes ImageContent.Data on the wire.
func pngResult(data []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}},
	}
}

// decodeImage accepts either bare base64 or a data: URL, because an agent
// handed an image by a tool that emits data URLs would otherwise have to strip
// the prefix itself and will sometimes forget.
func decodeImage(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, app.Errorf(app.KindInvalid, "imageBase64 is required")
	}
	if strings.HasPrefix(encoded, "data:") {
		_, payload, found := strings.Cut(encoded, ",")
		if !found {
			return nil, app.Errorf(app.KindInvalid, "imageBase64 looks like a data URL but carries no comma")
		}
		encoded = payload
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, app.Errorf(app.KindInvalid, "imageBase64 is not valid base64")
	}
	return raw, nil
}
