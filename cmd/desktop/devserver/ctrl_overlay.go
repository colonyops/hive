package main

import (
	"errors"
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/internal/web/extractors"
)

// overlayRequest is the body of POST /_ctl/overlay and /_ctl/overlay/clear.
type overlayRequest struct {
	Repo string    `json:"repo"`
	Num  int       `json:"num"`
	Set  Mutations `json:"set"`
}

func (r overlayRequest) Validate() error {
	return criterio.ValidateStruct(
		matcherErrors(Matcher{Repo: r.Repo, Num: r.Num}),
		criterio.Nest("set", validateMutations(r.Set)),
	)
}

func matcherErrors(m Matcher) error {
	return criterio.ValidateStruct(
		criterio.Run("repo", m.Repo, repoRef),
		criterio.Run("num", m.Num, criterio.Positive[int]()),
	)
}

func repoRef(val string) error {
	if !validRepo(val) {
		return errors.New(`must be "owner/name"`)
	}
	return nil
}

type overlayResponse struct {
	Item    string    `json:"item"`
	Overlay Mutations `json:"overlay"`
}

type clearResponse struct {
	Item    string `json:"item,omitempty"`
	Cleared bool   `json:"cleared"`
}

func (c *Control) SetOverlay(w http.ResponseWriter, r *http.Request) error {
	req, err := extractors.Body[overlayRequest](w, r)
	if err != nil {
		return err
	}
	match := Matcher{Repo: req.Repo, Num: req.Num}
	merged := c.store.Apply(match.Key(), req.Set)
	c.logger.Info().Str("item", match.Key()).Msg("overlay applied")
	return server.JSON(w, http.StatusOK, overlayResponse{Item: match.Key(), Overlay: merged})
}

func (c *Control) ClearOverlay(w http.ResponseWriter, r *http.Request) error {
	req, err := extractors.Body[overlayRequest](w, r)
	if err != nil {
		return err
	}
	match := Matcher{Repo: req.Repo, Num: req.Num}
	c.store.Clear(match.Key())
	return server.JSON(w, http.StatusOK, clearResponse{Item: match.Key(), Cleared: true})
}

func (c *Control) ClearAllOverlays(w http.ResponseWriter, _ *http.Request) error {
	c.store.ClearAll()
	c.logger.Info().Msg("all overlays cleared")
	return server.JSON(w, http.StatusOK, clearResponse{Cleared: true})
}
