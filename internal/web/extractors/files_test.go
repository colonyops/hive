package extractors

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesBoundsAndTemporaryCleanup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files int
		field string
		text  bool
		limit int64
		valid bool
	}{
		{"valid spilled upload", 1, "images", false, 3 << 20, true},
		{"streamed body exceeds limit", 1, "images", false, 1 << 20, false},
		{"too many files", 2, "images", false, 5 << 20, false},
		{"unexpected file field", 1, "other", false, 3 << 20, false},
		{"unexpected text field", 1, "images", true, 3 << 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			for range tc.files {
				part, err := form.CreateFormFile(tc.field, "image.png")
				require.NoError(t, err)
				_, err = part.Write(make([]byte, 2<<20))
				require.NoError(t, err)
			}
			if tc.text {
				require.NoError(t, form.WriteField("extra", "value"))
			}
			require.NoError(t, form.Close())
			r := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(&body))
			r.Header.Set("Content-Type", form.FormDataContentType())
			files, cleanup, err := Files(httptest.NewRecorder(), r, "images", tc.limit, 1)
			if tc.valid {
				require.NoError(t, err)
				require.Len(t, files, 1)
				entries, err := os.ReadDir(tmp)
				require.NoError(t, err)
				require.NotEmpty(t, entries)
			} else {
				require.Error(t, err)
			}
			cleanup()
			entries, err := os.ReadDir(tmp)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}
