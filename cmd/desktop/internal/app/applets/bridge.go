package applets

import _ "embed"

//go:embed bridge.js
var bridgeScript string

func BridgeScript() string { return bridgeScript }
