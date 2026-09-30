package ghclient

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ItemRef identifies one issue or pull request to look up in ItemStates.
type ItemRef struct {
	Owner  string
	Name   string
	Number int
}

// ItemState is the lifecycle state of one ItemRef as of the query.
type ItemState struct {
	Author    string
	Number    int
	State     string // "open", "closed", or "merged"
	UpdatedAt time.Time
	Found     bool // false when the alias resolved null: repo gone or private, or no such number
}

const itemStateChunk = 100

// ItemStates resolves the current lifecycle state of each ref via a batched,
// aliased GraphQL query, chunked at itemStateChunk refs per request. out is
// always len(refs), including alongside an error: a failing chunk stops the
// rest, and the states already resolved are still in out.
//
// A ref whose repository is deleted or private resolves to a null alias plus a
// NOT_FOUND error, which is tolerated: that ref comes back Found:false while
// the rest of its chunk resolves normally, so one gone repo cannot strand the
// whole batch.
func (c *Client) ItemStates(ctx context.Context, refs []ItemRef) ([]ItemState, error) {
	if len(refs) == 0 {
		return []ItemState{}, nil
	}

	out := make([]ItemState, len(refs))
	base := 0
	for chunk := range slices.Chunk(refs, itemStateChunk) {
		doc, variables := buildItemStateQuery(chunk)
		var data map[string]gqlItemResult
		if err := c.postGraphQL(ctx, doc, variables, &data, true); err != nil {
			return out, err
		}
		for i := range chunk {
			result, ok := data[fmt.Sprintf("r%d", i)]
			if !ok || result.Item == nil {
				continue
			}
			out[base+i] = ItemState{
				Number:    result.Item.Number,
				State:     strings.ToLower(result.Item.State),
				UpdatedAt: result.Item.UpdatedAt,
				Found:     true,
			}
			if result.Item.Author != nil {
				out[base+i].Author = result.Item.Author.Login
			}
		}
		base += len(chunk)
	}
	return out, nil
}

// buildItemStateQuery constructs the aliased GraphQL document and its
// variable map for a chunk of item refs. Alias rN and variables $oN/$nN/$iN
// correspond to refs[N]; owner, name, and number are always passed through
// variables, never interpolated into the document text.
func buildItemStateQuery(refs []ItemRef) (doc string, variables map[string]any) {
	variables = make(map[string]any, len(refs)*3)
	var b strings.Builder
	b.WriteString("query (")
	for i, ref := range refs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "$o%d: String!, $n%d: String!, $i%d: Int!", i, i, i)
		variables[fmt.Sprintf("o%d", i)] = ref.Owner
		variables[fmt.Sprintf("n%d", i)] = ref.Name
		variables[fmt.Sprintf("i%d", i)] = ref.Number
	}
	b.WriteString(") {\n")
	for i := range refs {
		fmt.Fprintf(&b, "  r%d: repository(owner: $o%d, name: $n%d) {\n", i, i, i)
		fmt.Fprintf(&b, "    issueOrPullRequest(number: $i%d) {\n", i)
		// repository { nameWithOwner } is unused by this client — results map
		// back by alias index — but it makes each node self-identifying for
		// cmd/devserver's overlay rewriter, which only sees the response body.
		// A future change to this selection set must not drop it.
		b.WriteString(`      __typename
      ... on Issue { number state updatedAt author { login } repository { nameWithOwner } }
      ... on PullRequest { number state updatedAt author { login } repository { nameWithOwner } }
    }
  }
`)
	}
	b.WriteString("}\n")
	return b.String(), variables
}

type gqlItemResult struct {
	Item *gqlItemNode `json:"issueOrPullRequest"`
}

type gqlItemNode struct {
	Author     *gqlSearchAuthor `json:"author"`
	Type       string           `json:"__typename"`
	Number     int              `json:"number"`
	State      string           `json:"state"`
	UpdatedAt  time.Time        `json:"updatedAt"`
	Repository gqlRepository    `json:"repository"`
}
