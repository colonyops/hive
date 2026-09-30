package app

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
)

// starterSeed is the graph a freshly seeded workspace begins with: a few
// sources.github nodes each wired to its own feed terminal, laid out in two
// columns (sources left, feeds right), plus a "Review requests" feed and a
// notify node behind a filter on the notifications source.
//
// That notify branch ships on by default deliberately: "tell me when I'm
// asked to review something" is the case a quiet feed cannot serve — the item
// sits unread until you happen to look — and it should not require
// hand-authoring a flow to get. A feed never interrupts, so the interrupt is
// its own notify terminal on a second branch off the filter.
//
// It lives here rather than in flow because it names a connector, and flow is
// connector-neutral: a flow's graph is built by whoever knows which connectors
// exist, which is this package.
//
// credential is the account the source nodes fetch as, as "github/<login>".
func starterSeed(credential string) flow.Seed {
	seeds := []struct {
		feedID, feedName, kind, query string
	}{
		{"my-open-prs", "My open PRs", "search", "is:open is:pr author:@me archived:false"},
		{"assigned", "Assigned", "search", "is:open assignee:@me archived:false"},
		{"notifications", "Notifications", "notifications", ""},
	}

	const notificationsSeedID = "notifications"

	out := flow.Seed{Layout: flow.Layout{Nodes: map[string]flow.NodePosition{}}}
	for i, seed := range seeds {
		srcID := seed.feedID + "-src"
		out.Nodes = append(out.Nodes,
			flow.Node{ID: srcID, Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: credential, Kind: seed.kind, Query: seed.query})},
			flow.Node{ID: seed.feedID, Type: "feed", Name: seed.feedName, Config: &flow.FeedConfig{}},
		)
		out.Wires = append(out.Wires, flow.Wire{From: srcID, To: seed.feedID})
		out.Layout.Nodes[srcID] = flow.NodePosition{X: 48, Y: 48 + i*96}
		out.Layout.Nodes[seed.feedID] = flow.NodePosition{X: 360, Y: 48 + i*96}

		if seed.feedID != notificationsSeedID {
			continue
		}
		// The filter takes a second branch off the same source rather than
		// sitting between it and the Notifications feed: everything still lands
		// there to read at leisure, and only review requests reach the branch
		// that interrupts. The feed collects them; the notify node alongside it
		// is what raises the banner.
		out.Nodes = append(out.Nodes,
			flow.Node{ID: "review-requests-filter", Type: "github-filter", Config: &flow.GithubFilterConfig{Reasons: []string{"review_requested"}}},
			flow.Node{ID: "review-requests", Type: "feed", Name: "Review requests", Config: &flow.FeedConfig{Icon: "eye"}},
			flow.Node{ID: "review-requests-notify", Type: "notify", Name: "Review requested", Config: &flow.NotifyConfig{
				Title: "Review requested",
				Body:  "{{ .Payload.repo }} #{{ .Payload.num }} · {{ .Payload.title }}",
			}},
		)
		out.Wires = append(out.Wires,
			flow.Wire{From: srcID, To: "review-requests-filter"},
			flow.Wire{From: "review-requests-filter", Out: 0, To: "review-requests"},
			flow.Wire{From: "review-requests-filter", Out: 0, To: "review-requests-notify"},
		)
		out.Layout.Nodes["review-requests-filter"] = flow.NodePosition{X: 360, Y: 48 + (i+1)*96}
		out.Layout.Nodes["review-requests"] = flow.NodePosition{X: 672, Y: 48 + (i+1)*96}
		out.Layout.Nodes["review-requests-notify"] = flow.NodePosition{X: 672, Y: 48 + (i+2)*96}
	}
	return out
}
