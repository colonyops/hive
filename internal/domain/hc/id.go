package hc

import "github.com/colonyops/hive/pkg/randid"

// GenerateID returns a short unique ID for an HC item.
func GenerateID() string {
	return "hc-" + randid.Generate(8)
}

// GenerateCommentID returns a short unique ID for an HC comment.
func GenerateCommentID() string {
	return "hcc-" + randid.Generate(8)
}
