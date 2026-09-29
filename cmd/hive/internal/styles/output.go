package styles

// CLIOutputStyler styles the headers that hooks and file copies print. It
// reads the package styles on each call, so it follows a theme change.
type CLIOutputStyler struct{}

func (CLIOutputStyler) Header(s string) string { return CommandHeaderStyle.Render(s) }

func (CLIOutputStyler) Muted(s string) string { return TextMutedStyle.Render(s) }

func (CLIOutputStyler) Text(s string) string { return TextForegroundStyle.Render(s) }
