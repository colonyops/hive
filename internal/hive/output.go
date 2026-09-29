package hive

// OutputStyler styles the headers that hooks and file copies print. The
// caller supplies it, so this package has no rendering dependency.
type OutputStyler interface {
	Header(s string) string
	Muted(s string) string
	Text(s string) string
}

// PlainStyler returns every string unchanged.
type PlainStyler struct{}

func (PlainStyler) Header(s string) string { return s }

func (PlainStyler) Muted(s string) string { return s }

func (PlainStyler) Text(s string) string { return s }
