// Package ghcli implements hive's built-in GitHub sources as cliengine
// drivers backed by the gh CLI.
//
// Each built-in source is a cliengine.Driver: static identity/picker
// properties plus the gh argv construction and JSON parsing behind Search
// (and, for DetailDrivers, FetchDetail). The shared cliengine engine executes
// drivers, so adding a new gh-backed source means writing a new driver (see
// issues.go and prs.go), not a new source implementation.
package ghcli

// ghAuthor is the author sub-object embedded in gh list JSON.
type ghAuthor struct {
	Login string `json:"login"`
}

// ghLabel is a single label sub-object embedded in gh list JSON.
type ghLabel struct {
	Name string `json:"name"`
}

// labelNames extracts non-empty label names.
func labelNames(labels []ghLabel) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		if label.Name != "" {
			names = append(names, label.Name)
		}
	}
	return names
}

// ghRef is a minimal issue/PR cross-reference: the PRs that would close an
// issue (closedByPullRequestsReferences) or the issues a PR closes
// (closingIssuesReferences).
type ghRef struct {
	Number int `json:"number"`
}

// assigneeSummary returns the first assignee login and the total count.
func assigneeSummary(assignees []ghAuthor) (login string, count int) {
	if len(assignees) == 0 {
		return "", 0
	}
	return assignees[0].Login, len(assignees)
}

// firstRef returns the first cross-reference's number and the total count.
// A zero number means there are no references.
func firstRef(refs []ghRef) (number, count int) {
	if len(refs) == 0 {
		return 0, 0
	}
	return refs[0].Number, len(refs)
}
