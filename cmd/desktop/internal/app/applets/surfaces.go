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

// Surfaces is every place an applet.yaml can ask to appear.
var Surfaces = []Surface{
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
