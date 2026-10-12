package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web/extractors"
)

func (s *api) listRecipients(w http.ResponseWriter, r *http.Request) error {
	rs, err := s.core.Accounts.ListRecipients(r.Context(), authctx.AccountFrom(r.Context()).Email)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.RecipientsFrom(rs))
}

func (s *api) createRecipient(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[wire.CreateRecipient](w, r)
	if err != nil {
		return err
	}
	rec, err := s.core.Accounts.AddRecipient(r.Context(), authctx.AccountFrom(r.Context()).Email, body.Recipient, body.Device)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, wire.RecipientFrom(rec))
}

func (s *api) deleteRecipient(w http.ResponseWriter, r *http.Request) error {
	if err := s.core.Accounts.RemoveRecipient(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
