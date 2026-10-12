package drive

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Block kinds. Kind decides which content field of a Block is set: markdown
// and html carry Body, link carries URL.
const (
	BlockKindMarkdown = "markdown"
	BlockKindLink     = "link"
	BlockKindHTML     = "html"
)

// Block is one entry on a canvas. An html block's Body is the agent's source
// verbatim; sanitizing happens on the way out, never on the way in, so
// read_canvas shows the agent what it wrote. Timestamps are unix
// milliseconds.
type Block struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body,omitempty"`
	URL       string `json:"url,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Canvas is one canvas document, blocks in display order. Space is the key
// of the space that owns it and Path its document path there. Session and
// HiveSession are the chat or the hive session that created it: provenance
// for labeling, never authorization, because a canvas belongs to its space,
// not to its author.
type Canvas struct {
	Space       string      `json:"space"`
	Path        string      `json:"path"`
	Title       string      `json:"title,omitempty"`
	Session     string      `json:"session"`
	HiveSession string      `json:"hiveSession,omitempty"`
	CreatedAt   int64       `json:"createdAt"`
	UpdatedAt   int64       `json:"updatedAt"`
	Frontmatter Frontmatter `json:"frontmatter,omitempty"`
	Blocks      []Block     `json:"blocks"`
}

// Author is who wrote a canvas first: a chat by its record id, or a hive
// session by its id. At most one is set; neither is an agent outside both.
type Author struct {
	Session     string
	HiveSession string
}

// UnmarshalJSON accepts numeric chat ids written before chats moved to UUIDs.
// The id is kept as the author and rewritten as a string on the next write.
func (c *Canvas) UnmarshalJSON(data []byte) error {
	type alias Canvas
	decoded := struct {
		Session json.RawMessage `json:"session"`
		*alias
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Session) == 0 || string(decoded.Session) == "null" {
		return nil
	}
	if err := json.Unmarshal(decoded.Session, &c.Session); err == nil {
		return nil
	}
	var legacy int64
	if err := json.Unmarshal(decoded.Session, &legacy); err != nil {
		return fmt.Errorf("decode canvas session: %w", err)
	}
	if legacy > 0 {
		c.Session = strconv.FormatInt(legacy, 10)
	}
	return nil
}

// CanvasMeta is one row of a canvas listing: what a picker needs without
// loading block content.
type CanvasMeta struct {
	Space       string `json:"space"`
	Path        string `json:"path"`
	Title       string `json:"title,omitempty"`
	Session     string `json:"session"`
	HiveSession string `json:"hiveSession,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	BlockCount  int    `json:"blockCount"`
}
