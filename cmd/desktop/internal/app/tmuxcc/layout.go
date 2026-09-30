package tmuxcc

import (
	"errors"
	"fmt"
	"strings"
)

// SplitKind says how a layout node divides its box among its cells.
type SplitKind string

const (
	// SplitNone marks a leaf: one pane filling the box.
	SplitNone SplitKind = ""
	// SplitLeftRight lays cells side by side — what `split-window -h` makes.
	SplitLeftRight SplitKind = "leftright"
	// SplitTopBottom stacks cells — what `split-window -v` makes.
	SplitTopBottom SplitKind = "topbottom"
)

// Layout is a window's pane tree as tmux lays it out, in cells. A leaf names
// a pane; a node divides its box among Cells, each a Layout of its own. Every
// cell carries its own box, so a renderer places a pane without walking the
// tree, and the one-cell gap between two siblings is tmux's border. tmux never
// nests a node inside one of the same kind, so the divider between two
// siblings always belongs to their parent.
type Layout struct {
	Pane   string
	Split  SplitKind
	X, Y   int
	Width  int
	Height int
	Cells  []Layout
}

var errLayout = errors.New("tmuxcc: malformed layout")

// maxLayoutDepth bounds the parser's recursion. tmux alternates split kinds
// down the tree, so a real layout is shallow; a runaway string is refused
// rather than followed.
const maxLayoutDepth = 32

// maxNumberDigits bounds a dimension, an offset or a pane id to what fits an
// int32: nothing tmux emits comes close, and a longer run of digits is a
// hostile string rather than a layout.
const maxNumberDigits = 9

// ParseLayout reads tmux's layout string: `<checksum>,<cell>`, where a cell is
// `WxH,X,Y` followed by `,<pane>` for a leaf, `{<cells>}` for a left-right
// split or `[<cells>]` for a top-bottom one. Pane ids in it are bare numbers;
// they come back carrying the `%` every other tmux surface spells them with.
func ParseLayout(s string) (Layout, error) {
	_, body, ok := strings.Cut(s, ",")
	if !ok {
		return Layout{}, fmt.Errorf("%w: no checksum in %q", errLayout, s)
	}
	p := layoutParser{src: body}
	root, err := p.cell(0)
	if err != nil {
		return Layout{}, err
	}
	if p.pos != len(p.src) {
		return Layout{}, fmt.Errorf("%w: trailing %q", errLayout, p.src[p.pos:])
	}
	return root, nil
}

type layoutParser struct {
	src string
	pos int
}

func (p *layoutParser) cell(depth int) (Layout, error) {
	if depth > maxLayoutDepth {
		return Layout{}, fmt.Errorf("%w: nested past %d levels", errLayout, maxLayoutDepth)
	}
	var l Layout
	var err error
	if l.Width, err = p.number(); err != nil {
		return Layout{}, err
	}
	if err := p.expect('x'); err != nil {
		return Layout{}, err
	}
	if l.Height, err = p.number(); err != nil {
		return Layout{}, err
	}
	if err := p.expect(','); err != nil {
		return Layout{}, err
	}
	if l.X, err = p.number(); err != nil {
		return Layout{}, err
	}
	if err := p.expect(','); err != nil {
		return Layout{}, err
	}
	if l.Y, err = p.number(); err != nil {
		return Layout{}, err
	}

	switch p.peek() {
	case ',':
		p.pos++
		pane, err := p.number()
		if err != nil {
			return Layout{}, err
		}
		l.Pane = fmt.Sprintf("%%%d", pane)
		return l, nil
	case '{':
		l.Split = SplitLeftRight
		l.Cells, err = p.cells('}', depth)
		return l, err
	case '[':
		l.Split = SplitTopBottom
		l.Cells, err = p.cells(']', depth)
		return l, err
	default:
		return Layout{}, fmt.Errorf("%w: expected a pane or a split at %d in %q", errLayout, p.pos, p.src)
	}
}

func (p *layoutParser) cells(closer byte, depth int) ([]Layout, error) {
	p.pos++ // the opener
	var cells []Layout
	for {
		cell, err := p.cell(depth + 1)
		if err != nil {
			return nil, err
		}
		cells = append(cells, cell)
		switch p.peek() {
		case ',':
			p.pos++
		case closer:
			p.pos++
			return cells, nil
		default:
			return nil, fmt.Errorf("%w: expected %q at %d in %q", errLayout, closer, p.pos, p.src)
		}
	}
}

func (p *layoutParser) number() (int, error) {
	start := p.pos
	n := 0
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		if p.pos-start >= maxNumberDigits {
			return 0, fmt.Errorf("%w: number too long at %d in %q", errLayout, start, p.src)
		}
		n = n*10 + int(p.src[p.pos]-'0')
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("%w: expected a number at %d in %q", errLayout, start, p.src)
	}
	return n, nil
}

func (p *layoutParser) expect(c byte) error {
	if p.peek() != c {
		return fmt.Errorf("%w: expected %q at %d in %q", errLayout, c, p.pos, p.src)
	}
	p.pos++
	return nil
}

func (p *layoutParser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

// Leaves lists every pane cell, left to right and top to bottom.
func (l Layout) Leaves() []Layout {
	if l.Split == SplitNone {
		if l.Pane == "" {
			return nil
		}
		return []Layout{l}
	}
	var leaves []Layout
	for _, cell := range l.Cells {
		leaves = append(leaves, cell.Leaves()...)
	}
	return leaves
}

// Panes lists the pane ids of every leaf, in Leaves order.
func (l Layout) Panes() []string {
	leaves := l.Leaves()
	panes := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		panes = append(panes, leaf.Pane)
	}
	return panes
}

// Equal reports whether two layouts describe the same tree over the same
// boxes.
func (l Layout) Equal(o Layout) bool {
	if l.Pane != o.Pane || l.Split != o.Split || l.X != o.X || l.Y != o.Y ||
		l.Width != o.Width || l.Height != o.Height || len(l.Cells) != len(o.Cells) {
		return false
	}
	for i := range l.Cells {
		if !l.Cells[i].Equal(o.Cells[i]) {
			return false
		}
	}
	return true
}

// String renders the tree back in tmux's own notation, minus the checksum —
// the form a test fixture and a log line both read best in.
func (l Layout) String() string {
	var b strings.Builder
	l.write(&b)
	return b.String()
}

func (l Layout) write(b *strings.Builder) {
	fmt.Fprintf(b, "%dx%d,%d,%d", l.Width, l.Height, l.X, l.Y)
	switch l.Split {
	case SplitNone:
		fmt.Fprintf(b, ",%s", strings.TrimPrefix(l.Pane, "%"))
	case SplitLeftRight:
		l.writeCells(b, '{', '}')
	case SplitTopBottom:
		l.writeCells(b, '[', ']')
	}
}

func (l Layout) writeCells(b *strings.Builder, open, closer byte) {
	b.WriteByte(open)
	for i, cell := range l.Cells {
		if i > 0 {
			b.WriteByte(',')
		}
		cell.write(b)
	}
	b.WriteByte(closer)
}
