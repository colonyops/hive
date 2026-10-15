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
