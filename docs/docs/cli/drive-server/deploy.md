---
icon: lucide/server
description: Run the Hive Drive Server in Docker, set its URL and mail, and put it behind a reverse proxy.
---

# Deploy the Drive Server

`hive-drive-server` serves Hive Drive documents to other people. People sign in with an email code, push documents from the [hive CLI](cli.md), and share them as [links](sharing.md). One server holds many accounts, and each account has its own SQLite database.

The image is `ghcr.io/colonyops/hive-drive-server`. It is published starting with the release after 0.64.0.

## Run it

The server needs a public URL and a way to send mail. It listens on port 8080 and reads two directories: `/config` for `drive-server.yaml` and `/data` for the databases.

```bash
docker run -d --name drive-server \
  -p 8080:8080 \
  -v drive-data:/data \
  -v "$PWD/config:/config" \
  -e HIVE_DRIVE_SERVER_BASE_URL=https://drive.example.com \
  -e HIVE_DRIVE_SERVER_SMTP_URL=smtps://drive@mail.example.com:465 \
  -e HIVE_DRIVE_SERVER_SMTP_FROM="Hive Drive <drive@example.com>" \
  -e HIVE_DRIVE_SERVER_SMTP_PASSWORD=env:SMTP_PASSWORD \
  -e SMTP_PASSWORD \
  ghcr.io/colonyops/hive-drive-server
```

The image's entrypoint is `hive-drive-server serve`, so any arguments after the image name are `serve` flags.

The same setup as a compose file:

```yaml
services:
  drive-server:
    image: ghcr.io/colonyops/hive-drive-server
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      HIVE_DRIVE_SERVER_BASE_URL: https://drive.example.com
      HIVE_DRIVE_SERVER_SMTP_URL: smtps://drive@mail.example.com:465
      HIVE_DRIVE_SERVER_SMTP_FROM: Hive Drive <drive@example.com>
      HIVE_DRIVE_SERVER_SMTP_PASSWORD: file:/run/secrets/smtp_password
    secrets:
      - smtp_password
    volumes:
      - drive-data:/data
      - ./config:/config

secrets:
  smtp_password:
    file: ./smtp_password.txt

volumes:
  drive-data:
```

`./config/drive-server.yaml` must list at least one [user policy](policies.md). With no policy, nobody can sign in.

`GET /healthz` returns `ok` and `GET /version` returns the build, for a health check.

## Settings

Each setting has a flag, an environment variable, and a key in `drive-server.yaml`. A flag wins over the environment, and the environment wins over the file. The flags belong to the `serve` command.

| Setting | Flag | Environment variable | File key | Default |
| --- | --- | --- | --- | --- |
| Public URL | `--base-url` | `HIVE_DRIVE_SERVER_BASE_URL` | `base_url` | none, required |
| Listen address | `--listen` | `HIVE_DRIVE_SERVER_LISTEN` | `listen` | `:8080` |
| Data directory | `--data-dir` | `HIVE_DRIVE_SERVER_DATA_DIR` | `data_dir` | `/data` |
| Config directory | `--config-dir` | `HIVE_DRIVE_SERVER_CONFIG_DIR` | none | `/config` |
| Largest document | `--max-document-size` | `HIVE_DRIVE_SERVER_MAX_DOCUMENT_SIZE` | `max_document_size` | `5MiB` |
| Trusted proxies | `--trusted-proxies` | `HIVE_DRIVE_SERVER_TRUSTED_PROXIES` | `trusted_proxies` | none |
| SMTP server | none | `HIVE_DRIVE_SERVER_SMTP_URL` | `smtp.url` | none |
| SMTP sender | none | `HIVE_DRIVE_SERVER_SMTP_FROM` | `smtp.from` | none |
| SMTP password | none | `HIVE_DRIVE_SERVER_SMTP_PASSWORD` | `smtp.password` | none |
| Print mail instead of sending it | `--dev-log-mail` | `HIVE_DRIVE_SERVER_DEV_LOG_MAIL` | none | off |
| Log level | `--log-level` | `HIVE_DRIVE_SERVER_LOG_LEVEL` | none | `info` |

- `base_url` must start with `https://`. `http://` is accepted only for `localhost` and other loopback addresses. The server puts this URL in sign-in mail and share links.
- A size is `unlimited` or a positive number with a unit: `KB`, `MB`, `GB`, `TB` (powers of 1000) or `KiB`, `MiB`, `GiB`, `TiB` (powers of 1024). A bare number is bytes.
- `trusted_proxies` takes CIDRs or single addresses. The flag and the variable take a comma-separated list.
- `smtp.url` is `smtp://[user@]host[:port]` or `smtps://[user@]host[:port]`. `smtp://` defaults to port 587 and upgrades with STARTTLS. `smtps://` defaults to port 465 and uses TLS from the first byte.
- `smtp.password` is a secret reference, never the password. A reference is `env:NAME`, `file:/path`, or `op://vault/item/field` for 1Password. The server rejects a value with none of these prefixes. It reads the password once at start, and only when `smtp.url` names a user.

The server refuses to start without a way to send mail. Set `smtp.url`, or pass `--dev-log-mail`.

## Behind a reverse proxy

Terminate TLS at a reverse proxy and forward to port 8080. The server limits sign-in attempts per client address: 3 codes per email per hour, 10 codes per address per hour, and 10 code checks per email in 10 minutes. Behind a proxy, every request seems to come from the proxy, so all users would share one limit.

Set `trusted_proxies` to the proxy's address range. The server then reads the client address from the right end of `X-Forwarded-For` and skips the trusted entries. Without `trusted_proxies`, the server ignores the header, so a client cannot choose its own limit bucket.

```yaml
trusted_proxies:
  - 172.18.0.0/16
```

## Try it locally

`--dev-log-mail` prints sign-in mail, codes included, to the server's standard output instead of sending it. Use it for a local try only.

```bash
docker run --rm -p 8080:8080 \
  -v "$PWD/config:/config" \
  -e HIVE_DRIVE_SERVER_BASE_URL=http://127.0.0.1:8080 \
  ghcr.io/colonyops/hive-drive-server --dev-log-mail
```

## Mount the config directory

Mount the directory that holds `drive-server.yaml`, not the file. Editors save by writing a new file and renaming it over the old one. A single-file mount keeps pointing at the old file, so the server never sees the change. The server watches the directory.

Kubernetes ConfigMap mounts are not supported.

## Back up

Everything is in the data directory: `control.db` and one `users/<id>.db` per account. The databases use write-ahead logging, so never copy the files of a running server with `cp` or a volume snapshot. A copy can miss committed data or fail to open.

Use one of these:

- Run `VACUUM INTO` on each file. It writes a consistent copy while the server runs.

    ```bash
    sqlite3 /data/control.db "VACUUM INTO '/backup/control.db'"
    for f in /data/users/*.db; do
      sqlite3 "$f" "VACUUM INTO '/backup/users/$(basename "$f")'"
    done
    ```

- Stop the container, copy the whole data directory, and start the container.

To restore, stop the server and put the files into an empty data directory.

## Upgrade

Pull the new image and restart the container. The server applies database migrations when it opens each file: `control.db` at start, and a user database the first time that account is used. Back up before you upgrade.

## Next

- [Choose who can sign in](policies.md)
- [Push, pull, and share from the CLI](cli.md)
