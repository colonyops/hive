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
	OwnerEmail string `json:"owner_email"`
	// OwnerName is the owner's display name, else the local part of their
	// email.
	OwnerName string   `json:"owner_name"`
	Document  Document `json:"document"`
	Body      string   `json:"body,omitempty"`
}

// FeatureComments in Me.Features means the server serves threads and
// presence.
const FeatureComments = "comments"

type Me struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Policy      string `json:"policy"`
	Quota       Quota  `json:"quota"`
	// Features names what this server supports, so a client can tell an
	// older server from a missing document.
	Features []string `json:"features"`
}

type UpdateMe struct {
	DisplayName string `json:"display_name"`
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

// Anchor is a source line range of a markdown document, 1-based and
// inclusive with front matter counted, and the text the author selected.
type Anchor struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Quote     string `json:"quote"`
}

// Author.ID and Author.Email are empty for an anonymous reader. Agent is the
// name of the API token that wrote it, and empty for a browser session.
type Author struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Agent string `json:"agent,omitempty"`
}

// Comment.Deleted marks a thread's first comment removed while replies
// remain; its Body and Author are then empty.
type Comment struct {
	ID        string     `json:"id"`
	ThreadID  string     `json:"thread_id"`
	Author    Author     `json:"author"`
	Body      string     `json:"body"`
	Deleted   bool       `json:"deleted,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at,omitempty"`
}

// Thread.Anchor is nil for a general thread. Outdated means a push removed
// the quoted text; Anchor then holds the lines and quote it last had.
// Comments are oldest first.
type Thread struct {
	ID         string     `json:"id"`
	Author     Author     `json:"author"`
	Anchor     *Anchor    `json:"anchor,omitempty"`
	Outdated   bool       `json:"outdated"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy *Author    `json:"resolved_by,omitempty"`
	Comments   []Comment  `json:"comments"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Viewer is who asked for a ThreadList. UserID is empty for an anonymous
// reader, and CanComment needs a signed-in principal with push scope.
type Viewer struct {
	UserID     string `json:"user_id,omitempty"`
	IsOwner    bool   `json:"is_owner"`
	CanComment bool   `json:"can_comment"`
}

// ThreadList is what GET .../threads returns: general threads first, then
// by start line.
type ThreadList struct {
	DocumentSHA256 string   `json:"document_sha256"`
	Viewer         Viewer   `json:"viewer"`
	Threads        []Thread `json:"threads"`
}

// CreateThread.DocumentSHA256 is the sha256 of the body the client rendered.
// A thread on any other version is refused with a conflict.
type CreateThread struct {
	DocumentSHA256 string  `json:"document_sha256"`
	Anchor         *Anchor `json:"anchor,omitempty"`
	Body           string  `json:"body"`
}

type CommentBody struct {
	Body string `json:"body"`
}

type ThreadStateUpdate struct {
	Resolved bool `json:"resolved"`
}

// LineRange is a viewer's selection in presence. It is lines only, so
// presence never carries document text.
type LineRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// PresenceColors is the size of the presence palette.
const PresenceColors = 12

// PresenceUpdate is a heartbeat. Token is the secret from the stream's hello
// event. Name and Color count only for an anonymous viewer; Color is an
// index below PresenceColors.
type PresenceUpdate struct {
	Token     string     `json:"token"`
	Name      string     `json:"name,omitempty"`
	Color     int        `json:"color"`
	Selection *LineRange `json:"selection,omitempty"`
}

// PresenceViewer.ID is the stream's public id. Self is set only in the list
// sent to that viewer's own stream.
type PresenceViewer struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Color     int        `json:"color"`
	SignedIn  bool       `json:"signed_in"`
	Self      bool       `json:"self,omitempty"`
	Selection *LineRange `json:"selection,omitempty"`
}

type PresenceSnapshot struct {
	Viewers []PresenceViewer `json:"viewers"`
}

// StreamHello is the first event of a document stream.
type StreamHello struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type DocumentEvent struct {
	SHA256 string `json:"sha256"`
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
