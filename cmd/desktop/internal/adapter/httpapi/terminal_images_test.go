package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalImageUploadAuthenticationAndValidation(t *testing.T) {
	h := newTerminalHarness(t)
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	for _, tc := range []struct {
		name, token, field string
		data               []byte
		status             int
	}{
		{"unauthorized", "", "images", raw.Bytes(), http.StatusUnauthorized},
		{"wrong token", "bad", "images", raw.Bytes(), http.StatusUnauthorized},
		{"wrong field", testToken, "other", raw.Bytes(), http.StatusBadRequest},
		{"invalid image", testToken, "images", []byte("not an image"), http.StatusBadRequest},
		{"valid", testToken, "images", raw.Bytes(), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			part, err := form.CreateFormFile(tc.field, "../../untrusted.png")
			require.NoError(t, err)
			_, err = part.Write(tc.data)
			require.NoError(t, err)
			require.NoError(t, form.Close())
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.server.URL+"/api/terminal/images/upload", &body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", form.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+tc.token)
			resp, err := h.server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, tc.status, resp.StatusCode)
			if tc.status == http.StatusOK {
				var result terminalImagesResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
				require.Len(t, result.Pastes, 1)
				assert.NotContains(t, result.Pastes[0], "untrusted")
			}
		})
	}
}

func TestTerminalImageControlRequiresAuthBeforeParsing(t *testing.T) {
	h := newTerminalHarness(t)
	for _, path := range []string{"images/paths", "images/upload", "panes/paste"} {
		resp := h.post(t, "/api/terminal/"+path, "", "invalid")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		_ = resp.Body.Close()
	}
	resp := h.post(t, "/api/terminal/panes/paste", testToken, terminalPasteRequest{Slug: "gone", PaneID: "%42", Text: "image.png "})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}
