package wailsui

import (
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
)

// SpaceWire is one drive space. Key is the identity every DriveService call
// takes; Kind is repository, workspace, or shared.
type SpaceWire struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// EntryWire is one row of a space's tree. Path is relative to the space root
// with forward slashes, "" for the root itself. Kind is dir, markdown, html,
// canvas, or file. Created is the zero time where the file system records no
// birth time. Front and Children are never null.
type EntryWire struct {
	Path     string         `json:"path"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Size     int64          `json:"size"`
	Modified time.Time      `json:"modified"`
	Created  time.Time      `json:"created"`
	Title    string         `json:"title"`
	Front    map[string]any `json:"front"`
	Children []EntryWire    `json:"children"`
}

// HitWire is one search result; Entry carries no children.
type HitWire struct {
	Space SpaceWire `json:"space"`
	Entry EntryWire `json:"entry"`
}

// DocumentWire is one document for the reader. Markdown is set for a
// markdown document, HTML for an html one, Canvas for a canvas; a plain file
// carries only its entry.
type DocumentWire struct {
	Space    SpaceWire   `json:"space"`
	Entry    EntryWire   `json:"entry"`
	Markdown string      `json:"markdown"`
	HTML     string      `json:"html"`
	Canvas   *CanvasWire `json:"canvas"`
}

// CanvasWire is one canvas with its html blocks sanitized. Timestamps are
// unix milliseconds, as the canvas file stores them.
type CanvasWire struct {
	Space       string         `json:"space"`
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Session     string         `json:"session"`
	HiveSession string         `json:"hiveSession"`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Frontmatter map[string]any `json:"frontmatter"`
	Blocks      []BlockWire    `json:"blocks"`
}

type BlockWire struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func spaceWireOf(s drive.Space) SpaceWire {
	return SpaceWire{Kind: string(s.Kind), Key: s.Key, Name: s.Name}
}

func spaceWiresOf(spaces []drive.Space) []SpaceWire {
	out := make([]SpaceWire, 0, len(spaces))
	for _, s := range spaces {
		out = append(out, spaceWireOf(s))
	}
	return out
}

func entryWireOf(e drive.Entry) EntryWire {
	children := make([]EntryWire, 0, len(e.Children))
	for _, c := range e.Children {
		children = append(children, entryWireOf(c))
	}
	return EntryWire{
		Path: e.Path, Name: e.Name, Kind: string(e.Kind), Size: e.Size,
		Modified: e.Modified, Created: e.Created, Title: e.Title,
		Front: frontOf(e.Front), Children: children,
	}
}

func hitWiresOf(hits []drivesvc.Hit) []HitWire {
	out := make([]HitWire, 0, len(hits))
	for _, h := range hits {
		out = append(out, HitWire{Space: spaceWireOf(h.Space), Entry: entryWireOf(h.Entry)})
	}
	return out
}

func documentWireOf(d app.DocumentView) DocumentWire {
	out := DocumentWire{Space: spaceWireOf(d.Space), Entry: entryWireOf(d.Entry), Markdown: d.Markdown, HTML: d.HTML}
	if d.Canvas != nil {
		c := canvasWireOf(*d.Canvas)
		out.Canvas = &c
	}
	return out
}

func canvasWireOf(c drive.Canvas) CanvasWire {
	blocks := make([]BlockWire, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		blocks = append(blocks, BlockWire{
			ID: b.ID, Kind: b.Kind, Title: b.Title, Body: b.Body, URL: b.URL,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		})
	}
	return CanvasWire{
		Space: c.Space, Path: c.Path, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Frontmatter: frontOf(c.Frontmatter), Blocks: blocks,
	}
}

func frontOf(fm drive.Frontmatter) map[string]any {
	if fm == nil {
		return map[string]any{}
	}
	return fm
}
