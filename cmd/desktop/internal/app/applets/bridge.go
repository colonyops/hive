package applets

import _ "embed"

//go:embed bridge.js
var bridgeScript string

// BridgeScript is the script served in the head of every html applet page.
func BridgeScript() string { return bridgeScript }
