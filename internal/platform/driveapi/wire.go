// Package driveapi holds the wire types of the Hive Drive server API, shared
// by the server and its clients so the two cannot drift.
package driveapi

import (
	"net/url"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
)

// Document is an element of the JSON arrays that GET /api/v1/{space},
// GET /api/v1/recent?limit= (newest first, default 50, at most 200), and
// GET /api/v1/shared-links (every document with a link, newest first) return.
//
//tygo:emit export type DocumentKind = "markdown" | "html"
type Document struct {
	ID    string `json:"id"`
	Space string `json:"space"`
	Path  string `json:"path"`
	// Title is the frontmatter title, else the first "# " heading, else the
	// file name without its extension.
	Title     string     `json:"title"`
	Kind      drive.Kind `json:"kind"             tstype:"DocumentKind"`
	SHA256    string     `json:"sha256"`
	Size      int64      `json:"size"`
	UpdatedAt time.Time  `json:"updated_at"`
	Shared    *Share     `json:"shared,omitempty"`
}

// Snippet markers wrap each matched run in SearchHit.Snippet. They are
// private-use characters, so a client escapes the text and then turns the
// markers into highlight elements without parsing HTML from the server.
const (
	SnippetMatchStart = "\ue000"
	SnippetMatchEnd   = "\ue001"
)

// SearchHit is one result of GET /api/v1/search. Title matches rank above
// body matches.
type SearchHit struct {
	Document Document `json:"document"`
	Snippet  string   `json:"snippet"`
}

type SearchResults struct {
	Hits []SearchHit `json:"hits"`
}

type SpaceSummary struct {
	Space     string          `json:"space"`
	Kind      drive.SpaceKind `json:"kind"      tstype:"'repository' | 'workspace' | 'shared'"`
	Documents int             `json:"documents"`
	Bytes     int64           `json:"bytes"`
}

// ShareExpiryNone, as ShareRequest.ExpiresIn, removes a link's expiry.
const ShareExpiryNone = "none"

// ShareRequest.ExpiresIn is "" (keep the link's expiry), "none", "1d", "7d",
// or "30d".
type ShareRequest struct {
	ExpiresIn string `json:"expires_in,omitempty"`
}

//tygo:emit export type ShareExpiry = "" | "1d" | "7d" | "30d"
type Share struct {
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// ExpiresIn is the preset behind ExpiresAt: "1d", "7d", "30d", or "" for
	// no expiry.
	ExpiresIn string `json:"expires_in" tstype:"ShareExpiry"`
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
	Limit *int64 `json:"limit" tstype:"number | null,required"`
}

type SignInStart struct {
	Email string `json:"email"`
}

type SignInVerify struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// CreateToken.Scope is "read" or "push". ExpiresIn is "", "1d", "7d", "30d",
// "90d", or "365d"; "" never expires.
//
//tygo:emit export type TokenScope = "read" | "push"
type CreateToken struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"                tstype:"TokenScope"`
	ExpiresIn string `json:"expires_in,omitempty"`
}

type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"                  tstype:"TokenScope"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreatedToken is the only response that carries the secret.
type CreatedToken struct {
	Secret string `json:"secret"`
	Token  `tstype:",extends"`
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
