package core

import "time"

// Sign-in limits. Start has two buckets so one address cannot be mailed
// without end and one client cannot spray many addresses.
const (
	StartPerEmail        = 3
	StartPerEmailWindow  = time.Hour
	StartPerIP           = 10
	StartPerIPWindow     = time.Hour
	VerifyPerEmail       = 10
	VerifyPerEmailWindow = 10 * time.Minute
)

// Comment limits. Comments do not count toward the owner's storage quota,
// because other people write them; these caps bound them instead.
var DefaultCommentCaps = CommentCaps{
	ThreadsPerDocument:   500,
	CommentsPerThread:    200,
	BodyBytesPerDocument: 2 << 20,
}

const (
	MaxCommentBytes = 8 << 10
	MaxQuoteRunes   = 2000
	MaxDisplayName  = 64
)

// Comment and presence request limits.
const (
	CommentWritesPerUser = 30
	StreamOpensPerIP     = 30
	PresencePerToken     = 120
	PresencePerIP        = 600
	CommentLimitWindow   = time.Minute
)
