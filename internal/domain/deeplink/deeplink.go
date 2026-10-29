// Package deeplink defines hive:// URLs, the public address of a place in Hive
// Desktop. Other programs build them, the operating system hands them to the
// app, and the app routes to the place they name. The URL is the contract; the
// desktop's internal routes are not, and they can change under it.
//
//	hive://code/<slug>[?server=<name>][&window=<id>][&canvas=<name>]
//	hive://chats/<workspace>[/<chat>][?canvas=<name>]
//	hive://inbox/<profile>[/<item>][?feed=<id>]
//	hive://canvas/<name>?owner=<workspace | owner/repo | @global>
//	hive://drive/<space-key>[/<path>]
//
// A code link without server names a session on this machine. server is a
// name under servers: in config.yaml, or "local".
//
// A drive link's space key is "shared", "workspaces/<name>", or
// "<owner>/<repo>", and the segments after it are the document or folder
// path inside that space. A drive link with no path opens the space.
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

	"github.com/colonyops/hive/internal/domain/drive"
)

// Scheme is the URL scheme Hive Desktop registers with the operating system.
const Scheme = "hive"

// Area is the part of Hive Desktop a link opens.
//
// ENUM(code, chats, inbox, canvas, drive)
type Area string

// Location is one place in Hive Desktop. Which fields apply depends on Area:
//
//	code    Slug, Server, Window, Canvas
//	chats   Workspace, Chat, Canvas
//	inbox   Profile, Item, Feed
//	canvas  Canvas, Owner
//	drive   Space, Path
//
// Slug is a tmux session name, so a code link also reaches a session hive did
// not create. Canvas on a code or chats link opens the pane beside it; on a
// canvas link it is the canvas the full-page view opens.
type Location struct {
	Area      Area   `json:"area"`
	Slug      string `json:"slug,omitempty"`
	Server    string `json:"server,omitempty"`
	Window    string `json:"window,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Chat      string `json:"chat,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Item      int64  `json:"item,omitempty"`
	Feed      string `json:"feed,omitempty"`
	Canvas    string `json:"canvas,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Space     string `json:"space,omitempty"`
	Path      string `json:"path,omitempty"`
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
		if len(segments) != 1 {
			return Location{}, errors.New("code links name a session slug")
		}
		loc.Slug = segments[0]
		loc.Server = q.Get("server")
		loc.Window = q.Get("window")
		loc.Canvas = q.Get("canvas")
	case AreaChats:
		if len(segments) < 1 || len(segments) > 2 {
			return Location{}, errors.New("chats links name a workspace and an optional chat")
		}
		loc.Workspace = segments[0]
		if len(segments) == 2 {
			loc.Chat = segments[1]
		}
		loc.Canvas = q.Get("canvas")
		if loc.Canvas != "" && loc.Chat == "" {
			return Location{}, errors.New("chats links open a canvas only beside a chat")
		}
	case AreaInbox:
		if len(segments) < 1 || len(segments) > 2 {
			return Location{}, errors.New("inbox links name a profile and an optional item")
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
		if len(segments) != 1 {
			return Location{}, errors.New("canvas links name a canvas")
		}
		loc.Canvas = segments[0]
		loc.Owner = q.Get("owner")
		if loc.Owner == "" {
			return Location{}, errors.New("canvas links need an owner")
		}
	case AreaDrive:
		space, path, err := driveTarget(segments)
		if err != nil {
			return Location{}, err
		}
		loc.Space, loc.Path = space, path
	}
	return loc, nil
}

// driveTarget splits a drive link's segments into the space key and the path
// inside it. The key's length follows from its first segment, so a path
// never needs escaping to be told apart from the key; an escaped slash inside
// a key segment is refused rather than read as a separator.
func driveTarget(segments []string) (space, path string, err error) {
	parsed, path, err := drive.SplitSpacePath(strings.Join(segments, "/"))
	if err != nil {
		return "", "", fmt.Errorf("drive links name a space: shared, workspaces/<name>, or <owner>/<repo>: %w", err)
	}
	keyLen := strings.Count(parsed.Key, "/") + 1
	if len(segments) < keyLen || strings.Join(segments[:keyLen], "/") != parsed.Key {
		return "", "", fmt.Errorf("drive link space %q: %w", parsed.Key, drive.ErrInvalidSpace)
	}
	if path != "" {
		if err := drive.ValidatePath(path); err != nil {
			return "", "", fmt.Errorf("drive link: %w", err)
		}
	}
	return parsed.Key, path, nil
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
