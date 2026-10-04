package extractors

import (
	"mime/multipart"
	"net/http"

	"github.com/colonyops/hive/cmd/desktop/internal/web"
)

// Files leaves at most 1 MiB in memory; callers remove the form's temporary
// files after closing the readers. Authentication must precede extraction.
func Files(w http.ResponseWriter, r *http.Request, field string, maxBytes int64, maxFiles int) ([]*multipart.FileHeader, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	err := r.ParseMultipartForm(1 << 20)
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	if err != nil {
		cleanup()
		return nil, func() {}, &web.BadRequestError{Msg: "invalid or oversized image upload", Err: err}
	}
	files := r.MultipartForm.File[field]
	if len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 || len(files) < 1 || len(files) > maxFiles {
		cleanup()
		return nil, func() {}, &web.BadRequestError{Msg: "upload must contain between 1 and 10 images and no other fields"}
	}
	return files, cleanup, nil
}
