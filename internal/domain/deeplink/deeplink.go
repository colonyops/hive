// Package deeplink defines hive:// URLs, the public address of a place in Hive
// Desktop. Other programs build them, the operating system hands them to the
// app, and the app routes to the place they name. The URL is the contract; the
// desktop's internal routes are not, and they can change under it.
//
//	hive://code/<slug>[?window=<id>][&canvas=<name>]
//	hive://chats/<workspace>[/<chat>][?canvas=<name>]
//	hive://inbox/<profile>[/<item>][?feed=<id>]
//	hive://canvas/<name>?owner=<workspace | owner/repo | @global>
//
// Unknown query parameters are ignored, so a newer link still opens in an
// older app.
package deeplink

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Scheme is the URL scheme Hive Desktop registers with the operating system.
const Scheme = "hive"

// GlobalCanvasOwner owns the canvases of agents outside every session and
// workspace.
const GlobalCanvasOwner = "@global"

// Area is the part of Hive Desktop a link opens.
//
// ENUM(code, chats, inbox, canvas)
type Area string

// Location is one place in Hive Desktop. Which fields apply depends on Area:
//
//	code    Slug, Window, Canvas
//	chats   Workspace, Chat, Canvas
//	inbox   Profile, Item, Feed
//	canvas  Canvas, Owner
//
// Slug is a tmux session name, so a code link also reaches a session hive did
// not create. Canvas on a code or chats link opens the pane beside it; on a
// canvas link it is the canvas the full-page view opens.
type Location struct {
	Area      Area   `json:"area"`
	Slug      string `json:"slug,omitempty"`
	Window    string `json:"window,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Chat      string `json:"chat,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Item      int64  `json:"item,omitempty"`
	Feed      string `json:"feed,omitempty"`
	Canvas    string `json:"canvas,omitempty"`
	Owner     string `json:"owner,omitempty"`
}

// Parse reads a hive:// URL into the Location it names.
func Parse(raw string) (Location, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Location{}, fmt.Errorf("parse link: %w", err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return Location{}, fmt.Errorf("link scheme %q is not %s", u.Scheme, Scheme)
	}
	if u.Opaque != "" || u.Host == "" {
		return Location{}, fmt.Errorf("link must start with %s://", Scheme)
	}
	area, err := ParseArea(u.Host)
	if err != nil {
		return Location{}, fmt.Errorf("unknown link area %q", u.Host)
	}
	segments, err := pathSegments(u)
	if err != nil {
		return Location{}, err
	}

	q := u.Query()
	loc := Location{Area: area}
	switch area {
	case AreaCode:
		if err := wantSegments(segments, 1, 1, "code links name a session slug"); err != nil {
			return Location{}, err
		}
		loc.Slug = segments[0]
		loc.Window = q.Get("window")
		loc.Canvas = q.Get("canvas")
	case AreaChats:
		if err := wantSegments(segments, 1, 2, "chats links name a workspace and an optional chat"); err != nil {
			return Location{}, err
		}
		loc.Workspace = segments[0]
		if len(segments) == 2 {
			loc.Chat = segments[1]
		}
		loc.Canvas = q.Get("canvas")
	case AreaInbox:
		if err := wantSegments(segments, 1, 2, "inbox links name a profile and an optional item"); err != nil {
			return Location{}, err
		}
		loc.Profile = segments[0]
		if len(segments) == 2 {
			item, err := strconv.ParseInt(segments[1], 10, 64)
			if err != nil || item <= 0 {
				return Location{}, fmt.Errorf("inbox item %q is not a positive number", segments[1])
			}
			loc.Item = item
		}
		loc.Feed = q.Get("feed")
	case AreaCanvas:
		if err := wantSegments(segments, 1, 1, "canvas links name a canvas"); err != nil {
			return Location{}, err
		}
		loc.Canvas = segments[0]
		loc.Owner = q.Get("owner")
	}
	return loc, loc.Validate()
}

// Validate reports whether l names a place a link can open: the fields its
// Area requires are set, and no field from another area is.
func (l Location) Validate() error {
	var errs []error
	require := func(value, name string) {
		if value == "" {
			errs = append(errs, fmt.Errorf("%s link needs a %s", l.Area, name))
		}
	}
	forbid := func(set bool, name string) {
		if set {
			errs = append(errs, fmt.Errorf("%s link cannot carry a %s", l.Area, name))
		}
	}

	switch l.Area {
	case AreaCode:
		require(l.Slug, "session slug")
		forbid(l.Workspace != "" || l.Chat != "", "chat")
		forbid(l.Profile != "" || l.Item != 0 || l.Feed != "", "inbox item")
		forbid(l.Owner != "", "canvas owner")
	case AreaChats:
		require(l.Workspace, "workspace")
		if l.Canvas != "" && l.Chat == "" {
			errs = append(errs, errors.New("chats link opens a canvas only beside a chat"))
		}
		forbid(l.Slug != "" || l.Window != "", "session")
		forbid(l.Profile != "" || l.Item != 0 || l.Feed != "", "inbox item")
		forbid(l.Owner != "", "canvas owner")
	case AreaInbox:
		require(l.Profile, "profile")
		if l.Item < 0 {
			errs = append(errs, fmt.Errorf("inbox item %d is not a positive number", l.Item))
		}
		forbid(l.Slug != "" || l.Window != "", "session")
		forbid(l.Workspace != "" || l.Chat != "", "chat")
		forbid(l.Canvas != "" || l.Owner != "", "canvas")
	case AreaCanvas:
		require(l.Canvas, "canvas name")
		require(l.Owner, "canvas owner")
		forbid(l.Slug != "" || l.Window != "", "session")
		forbid(l.Workspace != "" || l.Chat != "", "chat")
		forbid(l.Profile != "" || l.Item != 0 || l.Feed != "", "inbox item")
	default:
		errs = append(errs, fmt.Errorf("unknown link area %q", l.Area))
	}
	return errors.Join(errs...)
}

// URL renders l as a hive:// URL. It fails when l does not validate.
func (l Location) URL() (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}

	var segments []string
	q := url.Values{}
	set := func(key, value string) {
		if value != "" {
			q.Set(key, value)
		}
	}
	switch l.Area {
	case AreaCode:
		segments = []string{l.Slug}
		set("window", l.Window)
		set("canvas", l.Canvas)
	case AreaChats:
		segments = []string{l.Workspace, l.Chat}
		set("canvas", l.Canvas)
	case AreaInbox:
		segments = []string{l.Profile}
		if l.Item > 0 {
			segments = append(segments, strconv.FormatInt(l.Item, 10))
		}
		set("feed", l.Feed)
	case AreaCanvas:
		segments = []string{l.Canvas}
		set("owner", l.Owner)
	}

	var b strings.Builder
	b.WriteString(Scheme + "://" + string(l.Area))
	for _, segment := range segments {
		if segment != "" {
			b.WriteString("/" + url.PathEscape(segment))
		}
	}
	if len(q) > 0 {
		b.WriteString("?" + q.Encode())
	}
	return b.String(), nil
}

// pathSegments splits the escaped path so a segment can carry an escaped
// slash, and rejects an empty segment rather than guessing what it meant.
func pathSegments(u *url.URL) ([]string, error) {
	path := strings.Trim(u.EscapedPath(), "/")
	if path == "" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	segments := make([]string, len(parts))
	for i, part := range parts {
		segment, err := url.PathUnescape(part)
		if err != nil {
			return nil, fmt.Errorf("link path: %w", err)
		}
		if segment == "" {
			return nil, errors.New("link path has an empty segment")
		}
		segments[i] = segment
	}
	return segments, nil
}

func wantSegments(segments []string, minimum, maximum int, usage string) error {
	if len(segments) < minimum || len(segments) > maximum {
		return errors.New(usage)
	}
	return nil
}
