package github

// issueListItem is the JSON shape of a single entry returned by
// `gh issue list --json number,title,state,author,labels,url`.
type issueListItem struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	State  string    `json:"state"`
	Author ghAuthor  `json:"author"`
	Labels []ghLabel `json:"labels"`
	URL    string    `json:"url"`
}

// issueDetail is the JSON shape returned by
// `gh issue view <id> --json number,title,body,url,state`.
type issueDetail struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

// ghAuthor is the author sub-object embedded in gh issue JSON.
type ghAuthor struct {
	Login string `json:"login"`
}

// ghLabel is a single label sub-object embedded in gh issue JSON.
type ghLabel struct {
	Name string `json:"name"`
}
