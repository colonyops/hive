package styles

import (
	"image/color"

	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"github.com/lucasb-eyer/go-colorful"
)

func colorHexPtr(c color.Color) *string {
	if c == nil {
		return nil
	}
	cc, ok := colorful.MakeColor(c)
	if !ok {
		return nil
	}
	hex := cc.Hex()
	return &hex
}

// GlamourStyle returns a Glamour style config derived from the active theme.
func GlamourStyle() glamouransi.StyleConfig {
	cfg := glamourstyles.DarkStyleConfig

	fg := colorHexPtr(ColorForeground)
	primary := colorHexPtr(ColorPrimary)
	secondary := colorHexPtr(ColorSecondary)
	muted := colorHexPtr(ColorMuted)
	surface := colorHexPtr(ColorSurface)

	cfg.Document.Color = fg

	cfg.Paragraph.Color = fg

	cfg.Heading.Color = primary
	cfg.H1.Color = fg
	cfg.H1.BackgroundColor = surface
	cfg.H2.Color = primary
	cfg.H3.Color = primary
	cfg.H4.Color = primary
	cfg.H5.Color = primary
	cfg.H6.Color = primary

	cfg.BlockQuote.Color = muted
	cfg.HorizontalRule.Color = muted

	cfg.Link.Color = secondary
	cfg.LinkText.Color = secondary

	cfg.Code.Color = secondary
	cfg.CodeBlock.Color = muted

	cfg.Table.Color = fg

	return cfg
}
