package applets

// Surface is a place beyond its panel where an applet can show a page. A new one is a row
// here, a row in SURFACES (frontend stores/appletSurfaces.ts), a list in
// settings.AppletsSettings, and the host that mounts AppletFrame.
type Surface struct {
	Name string
	// Label finishes "which its applet.yaml shows in ...".
	Label string
	// Page is what the applet offers for this surface, if it offers anything.
	Page func(Applet) (string, bool)
}

func ownEntry(a Applet) (string, bool) { return a.Entry, true }

// Surfaces is every place the user can put an applet. Every applet offers its own entry for
// the + menu; only the title bar asks applet.yaml for a page.
var Surfaces = []Surface{
	{Name: "tab", Label: "a session's + menu", Page: ownEntry},
	{Name: "page", Label: "a session's + menu", Page: ownEntry},
	{
		Name:  "titlebar",
		Label: "the title bar",
		Page: func(a Applet) (string, bool) {
			if a.Titlebar == nil {
				return "", false
			}
			return a.Titlebar.Entry, true
		},
	},
}
