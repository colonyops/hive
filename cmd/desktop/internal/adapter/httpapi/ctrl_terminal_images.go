package httpapi

import (
	"io"
	"net/http"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/terminalimg"
	"github.com/colonyops/hive/cmd/desktop/internal/web/extractors"
	"github.com/hay-kot/httpkit/server"
)

type (
	terminalImagePathsRequest struct {
		Paths []string `json:"paths"`
	}
	terminalImagesResponse struct {
		Pastes []string `json:"pastes"`
	}
	terminalPasteRequest struct {
		Slug   string `json:"slug"`
		PaneID string `json:"paneId"`
		Text   string `json:"text"`
	}
)

func (ctrl *Controller) TerminalImagePaths(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalImagePathsRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	pastes, err := ctrl.core.TerminalImages.Prepare(r.Context(), body.Paths)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalImagesResponse{Pastes: pastes})
}

func (ctrl *Controller) TerminalImageUpload(w http.ResponseWriter, r *http.Request) error {
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return err
	}
	files, cleanup, err := extractors.Files(w, r, "images", terminalimg.MaxBatchBytes+(1<<20), terminalimg.MaxFiles)
	if err != nil {
		return err
	}
	defer cleanup()
	readers := make([]io.Reader, 0, len(files))
	for _, file := range files {
		if file.Size > terminalimg.MaxFileBytes {
			return app.Errorf(app.KindInvalid, "Each image must be at most 20 MiB.")
		}
		input, err := file.Open()
		if err != nil {
			return app.Wrap(err, app.KindInternal, "Could not read the uploaded image.")
		}
		defer func() { _ = input.Close() }()
		readers = append(readers, input)
	}
	pastes, err := ctrl.core.TerminalImages.Save(r.Context(), readers)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalImagesResponse{Pastes: pastes})
}

func (ctrl *Controller) TerminalPaste(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalPasteRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if body.Slug == "" || body.PaneID == "" || body.Text == "" {
		return app.Errorf(app.KindInvalid, "A session, pane and paste text are required.")
	}
	if err := ctrl.core.Terminals.Paste(r.Context(), body.Slug, body.PaneID, []byte(body.Text)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
