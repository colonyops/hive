-- Drive uploads

-- name: UpsertDriveUpload :exec
INSERT INTO drive_uploads (id, server, space, path, sha256, uploaded_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (server, space, path) DO UPDATE SET
    sha256 = excluded.sha256,
    uploaded_at = excluded.uploaded_at;

-- name: ListDriveUploads :many
SELECT * FROM drive_uploads WHERE server = ? AND space = ? ORDER BY path ASC;
