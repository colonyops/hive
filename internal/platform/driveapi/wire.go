// Package driveapi holds the wire types of the Hive Drive server API, shared
// by the server and its clients so the two cannot drift.
package driveapi

import (
	"net/url"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
)

type Document struct {
	ID        string     `json:"id"`
	Space     string     `json:"space"`
	Path      string     `json:"path"`
	Kind      drive.Kind `json:"kind"`
	SHA256    string     `json:"sha256"`
	Size      int64      `json:"size"`
	UpdatedAt time.Time  `json:"updated_at"`
	Shared    *Share     `json:"shared,omitempty"`
}

type SpaceSummary struct {
	Space     string          `json:"space"`
	Kind      drive.SpaceKind `json:"kind"`
	Documents int             `json:"documents"`
	Bytes     int64           `json:"bytes"`
}

// ShareRequest.ExpiresIn is "", "1d", "7d", or "30d".
type ShareRequest struct {
	ExpiresIn string `json:"expires_in,omitempty"`
}

type Share struct {
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// PublicDocument is what a share link serves to a reader who is not signed
// in. Body is set for markdown only; HTML is served from the raw route.
type PublicDocument struct {
	OwnerEmail string   `json:"owner_email"`
	Document   Document `json:"document"`
	Body       string   `json:"body,omitempty"`
}

type Me struct {
	Email  string `json:"email"`
	Policy string `json:"policy"`
	Quota  Quota  `json:"quota"`
}

// Quota.Limit is nil for an unlimited policy.
type Quota struct {
	Used  int64  `json:"used"`
	Limit *int64 `json:"limit"`
}

// ErrorBody is the JSON shape of every error response. It matches
// internal/web.ErrorBody, which a platform package may not import.
type ErrorBody struct {
	Kind    string            `json:"kind"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// SpacePrefix is the route prefix of a space, without a trailing slash.
func SpacePrefix(s drive.Space) string {
	switch s.Kind {
	case drive.SpaceKindShared:
		return "/api/v1/shared"
	case drive.SpaceKindWorkspace:
		return "/api/v1/workspaces/" + url.PathEscape(s.Name)
	default:
		owner, repo, _ := strings.Cut(s.Key, "/")
		return "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	}
}

// DocPath is the route of one document. A document path is escaped per
// segment, so a space, "#", or "?" in a name stays part of the path.
func DocPath(s drive.Space, path string) string {
	return SpacePrefix(s) + "/docs/" + escapePath(path)
}

func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}
