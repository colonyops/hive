package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web/extractors"
)

func (s *api) listSources(w http.ResponseWriter, r *http.Request) error {
	views, err := s.core.Sources.List(r.Context(), authctx.AccountFrom(r.Context()).Email)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.SourcesFrom(views))
}

func (s *api) createSource(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[wire.CreateSource](w, r)
	if err != nil {
		return err
	}
	src, token, err := s.core.Sources.Create(r.Context(), authctx.AccountFrom(r.Context()).Email, body.Name, body.Secret)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, wire.CreatedSource{
		Source:     wire.SourceFrom(relay.SourceView{Source: src}),
		IngressURL: s.core.Sources.IngressURL(token),
	})
}

func (s *api) getSource(w http.ResponseWriter, r *http.Request) error {
	v, err := s.core.Sources.Get(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id"))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.SourceFrom(v))
}

func (s *api) updateSource(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[wire.UpdateSource](w, r)
	if err != nil {
		return err
	}
	email, id := authctx.AccountFrom(r.Context()).Email, r.PathValue("id")
	if _, err := s.core.Sources.Update(r.Context(), email, id, body.Name, body.Secret); err != nil {
		return err
	}
	v, err := s.core.Sources.Get(r.Context(), email, id)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.SourceFrom(v))
}

func (s *api) deleteSource(w http.ResponseWriter, r *http.Request) error {
	if err := s.core.Sources.Delete(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *api) rotateSource(w http.ResponseWriter, r *http.Request) error {
	token, err := s.core.Sources.Rotate(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id"))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.IngressURL{IngressURL: s.core.Sources.IngressURL(token)})
}
