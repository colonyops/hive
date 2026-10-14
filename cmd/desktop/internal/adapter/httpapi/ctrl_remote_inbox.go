package httpapi

import (
	"net/http"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/hay-kot/httpkit/server"
)

type remoteFeed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type remoteInbox struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Feeds   []remoteFeed `json:"feeds"`
	Problem string       `json:"problem"`
}
type remoteInboxesResponse struct {
	Inboxes []remoteInbox `json:"inboxes"`
}

func (ctrl *Controller) RemoteInboxes(w http.ResponseWriter, r *http.Request) error {
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return err
	}
	out := remoteInboxesResponse{Inboxes: []remoteInbox{}}
	for _, status := range ctrl.core.Flows.Statuses(r.Context()) {
		inbox := remoteInbox{ID: status.ID, Name: status.Flow.Name, Feeds: []remoteFeed{}}
		if inbox.Name == "" {
			inbox.Name = status.ID
		}
		if !status.Valid {
			inbox.Problem = "This remote inbox has an invalid flow."
		} else {
			for _, node := range status.Flow.FeedNodes() {
				name := node.Name
				if name == "" {
					name = node.ID
				}
				inbox.Feeds = append(inbox.Feeds, remoteFeed{ID: status.Flow.FeedID(node.ID), Name: name})
			}
		}
		out.Inboxes = append(out.Inboxes, inbox)
	}
	return server.JSON(w, http.StatusOK, out)
}

type remoteFeedRequest struct {
	ProfileID string `json:"profileId"`
	FeedID    string `json:"feedId"`
}
type remoteInboxItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	SourceKind  string `json:"sourceKind"`
	SourceState string `json:"sourceState"`
	Unread      bool   `json:"unread"`
}
type remoteFeedResponse struct {
	Items []remoteInboxItem `json:"items"`
}

func (ctrl *Controller) RemoteFeed(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[remoteFeedRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	profile, err := ctrl.core.Flows.Get(r.Context(), body.ProfileID)
	if err != nil {
		return err
	}
	found := false
	for _, node := range profile.FeedNodes() {
		if profile.FeedID(node.ID) == body.FeedID {
			found = true
			break
		}
	}
	if !found {
		return app.Errorf(app.KindNotFound, "feed does not belong to this remote inbox")
	}
	items, err := ctrl.core.Inbox.ListByFeed(r.Context(), body.ProfileID, body.FeedID, 200)
	if err != nil {
		return err
	}
	out := remoteFeedResponse{Items: make([]remoteInboxItem, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, remoteInboxItem{ID: item.ID, Title: item.Title, SourceKind: item.SourceKind, SourceState: item.SourceState, Unread: item.Unread})
	}
	return server.JSON(w, http.StatusOK, out)
}
