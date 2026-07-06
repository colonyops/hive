// Package ghcli implements hive's built-in GitHub sources as cliengine
// drivers backed by the gh CLI.
package ghcli

type ghAuthor struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

func labelNames(labels []ghLabel) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		if label.Name != "" {
			names = append(names, label.Name)
		}
	}
	return names
}

// ghRef is an issue/PR cross-reference: the PRs that would close an issue
// (closedByPullRequestsReferences) or the issues a PR closes
// (closingIssuesReferences).
type ghRef struct {
	Number int `json:"number"`
}

func assigneeSummary(assignees []ghAuthor) (login string, count int) {
	if len(assignees) == 0 {
		return "", 0
	}
	return assignees[0].Login, len(assignees)
}

func firstRef(refs []ghRef) (number, count int) {
	if len(refs) == 0 {
		return 0, 0
	}
	return refs[0].Number, len(refs)
}
