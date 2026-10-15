---
kind: added
---

**Hive Drive Server**, a self-hosted server that shares Hive Drive documents with other people. Each person signs in with a code sent by email and gets their own drive, a web UI to browse, search, share, and delete documents, API tokens for the `hive drive` commands, and public links that expire. It ships as the `ghcr.io/colonyops/hive-drive-server` image for linux/amd64 and linux/arm64. Set `HIVE_DRIVE_SERVER_BASE_URL` and an SMTP server, then run the image with a volume on `/data`; `--dev-log-mail` prints mail to the log for a local try-out.
