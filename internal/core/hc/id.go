package hc

import "github.com/colonyops/hive/pkg/randid"

// GenerateID returns a short unique ID for an HC item or activity.
func GenerateID() string {
	return "hc-" + randid.Generate(8)
}
