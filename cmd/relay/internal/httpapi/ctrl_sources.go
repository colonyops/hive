package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	maxSourceName   = 80
	maxSourceSecret = 128
)

type sourceView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	HasSecret  bool      `json:"has_secret"`
	CreatedAt  time.Time `json:"created_at"`
	Deliveries int       `json:"deliveries"`
	Bytes      int64     `json:"bytes"`
}

func toSourceView(v relay.SourceView) sourceView {
	return sourceView{
		ID:         v.ID,
		Name:       v.Name,
		HasSecret:  v.SecretHash != "",
		CreatedAt:  v.CreatedAt,
		Deliveries: v.Deliveries,
		Bytes:      v.Bytes,
	}
}

type sourcesResponse struct {
	Sources []sourceView `json:"sources"`
}

type createdSourceResponse struct {
	sourceView
	IngressURL string `json:"ingress_url"`
}

type ingressURLResponse struct {
	IngressURL string `json:"ingress_url"`
}

type createSourceRequest struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

func (b createSourceRequest) Validate() error {
	return criterio.ValidateStruct(
		validateName(b.Name),
		criterio.Run("secret", b.Secret, secretRules...),
	)
}

type updateSourceRequest struct {
	Name   *string `json:"name"`
	Secret *string `json:"secret"`
}

func (b updateSourceRequest) Validate() error {
	var errs []error
	if b.Name != nil {
		errs = append(errs, validateName(*b.Name))
	}
	if b.Secret != nil {
		errs = append(errs, criterio.Run("secret", *b.Secret, secretRules...))
	}
	return criterio.ValidateStruct(errs...)
}

func validateName(name string) error {
	return criterio.Run("name", strings.TrimSpace(name), criterio.StrBetween(1, maxSourceName))
}

var secretRules = []criterio.Validator[string]{criterio.StrMax(maxSourceSecret), printableASCII}

func printableASCII(s string) error {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return errors.New("must be printable ASCII")
		}
	}
	return nil
}

func (s *api) listSources(w http.ResponseWriter, r *http.Request) error {
	views, err := s.core.Sources.List(r.Context(), accountFrom(r.Context()).Email)
	if err != nil {
		return err
	}
	out := make([]sourceView, 0, len(views))
	for _, v := range views {
		out = append(out, toSourceView(v))
	}
	return server.JSON(w, http.StatusOK, sourcesResponse{Sources: out})
}

func (s *api) createSource(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[createSourceRequest](w, r)
	if err != nil {
		return err
	}
	src, token, err := s.core.Sources.Create(r.Context(), accountFrom(r.Context()).Email, body.Name, body.Secret)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, createdSourceResponse{
		sourceView: toSourceView(relay.SourceView{Source: src}),
		IngressURL: s.core.Sources.IngressURL(token),
	})
}

func (s *api) getSource(w http.ResponseWriter, r *http.Request) error {
	v, err := s.core.Sources.Get(r.Context(), accountFrom(r.Context()).Email, r.PathValue("id"))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toSourceView(v))
}

func (s *api) updateSource(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[updateSourceRequest](w, r)
	if err != nil {
		return err
	}
	email, id := accountFrom(r.Context()).Email, r.PathValue("id")
	if _, err := s.core.Sources.Update(r.Context(), email, id, body.Name, body.Secret); err != nil {
		return err
	}
	v, err := s.core.Sources.Get(r.Context(), email, id)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toSourceView(v))
}

func (s *api) deleteSource(w http.ResponseWriter, r *http.Request) error {
	if err := s.core.Sources.Delete(r.Context(), accountFrom(r.Context()).Email, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *api) rotateSource(w http.ResponseWriter, r *http.Request) error {
	token, err := s.core.Sources.Rotate(r.Context(), accountFrom(r.Context()).Email, r.PathValue("id"))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, ingressURLResponse{IngressURL: s.core.Sources.IngressURL(token)})
}
