---
icon: lucide/server
description: Run your own Hive Relay with Docker, configure sign-in and limits, and connect Hive Desktop to it.
---

# Self-host Hive Relay

Hive Relay stores HTTP requests that senders post to an ingress URL, and Hive Desktop pulls them on its next refresh. Run your own when you want the relay on your domain, your data on your disk, and a small group of people (you and a few friends) using it.

The hosted instance at `https://relay.hivedesktop.com` needs none of this. See [Hive Relay](../inbox/sources.md#hive-relay) for how sources use it.

## Run it

The image is `ghcr.io/colonyops/hive-relay`, tagged with each release version and `latest`. It is distroless, runs as a nonroot user, listens on port 8080, and keeps its data in `/data`.

```yaml title="compose.yaml"
services:
  relay:
    image: ghcr.io/colonyops/hive-relay:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      RELAY_PUBLIC_URL: https://relay.example.com
      RELAY_ALLOWED_EMAILS: you@example.com,*@example.org
      RELAY_SMTP_URL: smtps://relay@smtp.example.com:465
      RELAY_SMTP_PASSWORD: env:RELAY_SMTP_SECRET
      RELAY_SMTP_SECRET: ${RELAY_SMTP_SECRET}
      RELAY_MAIL_FROM: Hive Relay <relay@example.com>
      RELAY_TRUSTED_PROXIES: 172.16.0.0/12
    volumes:
      - relay-data:/data

volumes:
  relay-data:
```

Put a TLS-terminating proxy such as Caddy or Traefik in front of the container and send `relay.example.com` to port 8080. Set `RELAY_TRUSTED_PROXIES` to the proxy's address range. Without it the relay ignores `X-Forwarded-For`, and every request appears to come from the proxy, so the per-IP rate limits apply to all senders together.

`GET /healthz` returns `ok` when the relay is up.

To try a relay without an operator step, `--mint` admits one email and prints an API key for it on the first line of stdout. Every start mints another key, so use it for a throwaway relay only:

```sh
docker run --rm -p 8080:8080 -e RELAY_PUBLIC_URL=http://localhost:8080 ghcr.io/colonyops/hive-relay --mint you@example.com
```

## Configure

Every setting is a flag, an environment variable, or a key in an optional `relay.yaml`. A flag or variable that is set wins over the file. Unknown keys in the file are an error.

| Variable | Flag | Default | Purpose |
| --- | --- | --- | --- |
| `RELAY_PUBLIC_URL` | `--public-url` | none, required | URL senders reach. Used to build ingress URLs. |
| `RELAY_LISTEN` | `--listen` | `:8080` | Address to listen on. |
| `RELAY_DATA_DIR` | `--data-dir` | `.` (`/data` in the image) | Directory that holds `relay.db`. |
| `RELAY_TRUSTED_PROXIES` | `--trusted-proxies` | none | Comma-separated CIDRs whose `X-Forwarded-For` is honoured. |
| `RELAY_LOG_LEVEL` | `--log-level` | `info` | `debug`, `info`, `warn`, or `error`. |
| `RELAY_CONFIG` | `--config` | none | Path to `relay.yaml`. |
| `RELAY_ALLOWED_EMAILS` | none | none | Comma-separated email globs. Wins over the file. |
| `RELAY_SMTP_URL` | none | none | `smtp://user@host:587` (STARTTLS), `smtps://user@host:465` (implicit TLS), or `log`. |
| `RELAY_SMTP_PASSWORD` | none | none | A secret reference: `env:NAME`, `file:/path`, or `op://...`. |
| `RELAY_MAIL_FROM` | none | none | Sender address for sign-in mail. |

`RELAY_SMTP_URL=log` prints sign-in codes to the relay's stdout instead of sending mail. Use it for development.

The same settings as `relay.yaml`, with the limit keys from [Limits and retention](#limits-and-retention):

```yaml title="relay.yaml"
listen: ":8080"
public_url: https://relay.example.com
store: sqlite
data_dir: /data
log_level: info
trusted_proxies: ["172.16.0.0/12"]

body_bytes: 262144
retention: 336h
deliveries_per_source: 1000
bytes_per_account: 67108864
sources_per_account: 50
concurrency: 256

allowed_emails: ["you@example.com", "*@example.org"]
users:
  - email: you@example.com
    password_hash: "$argon2id$..."

smtp:
  url: smtps://relay@smtp.example.com:465
  password: env:RELAY_SMTP_SECRET
  from: Hive Relay <relay@example.com>
```

Send `SIGHUP` to reload `allowed_emails` and `users` without a restart. Every other key needs a restart.

## Sign-in methods

A person can sign in three ways. The relay checks the allowlist and the user list on every request, so removing an email revokes its tokens at the next request.

- **Email code.** On when `smtp` is set and `allowed_emails` has at least one pattern. The relay mails a code to any address that matches a pattern.
- **Password.** On when `users:` has entries. Create the hash with `hive-relay hash-password`, which reads the password from stdin, and paste it as `password_hash`. The hash is argon2id.
- **API key.** Always on. Create one with `hive-relay tokens create`.

```sh
docker compose exec -T relay hive-relay hash-password
```

!!! warning "Empty lists admit nobody"
    With no allowlist pattern and no `users:` entry, nobody can sign in. The relay logs a warning at start. API keys still work.

`hash-password --password` is for scripts. The password then shows in the process list.

## Operator commands

Run these on the host with the same flags or environment as `serve`. They need the `sqlite` store, because they open the database directly.

```sh
docker compose exec relay hive-relay tokens create --email you@example.com --name laptop
docker compose exec relay hive-relay accounts list
docker compose exec relay hive-relay accounts delete --email you@example.com
```

`tokens create` prints the key once. Copy it then. `accounts delete` removes the account with its sources, deliveries, and tokens, and prints what it removed.

## Limits and retention

| Limit | Default | Tunable with |
| --- | --- | --- |
| Request body | 256 KiB | `RELAY_BODY_BYTES` |
| Retention | 14 days | `RELAY_RETENTION`, a Go duration such as `336h` |
| Deliveries kept per source | 1000 | `RELAY_DELIVERIES_PER_SOURCE` |
| Stored bytes per account | 64 MiB | `RELAY_BYTES_PER_ACCOUNT` |
| Sources per account | 50 | `RELAY_SOURCES_PER_ACCOUNT` |
| In-flight ingress requests | 256 | `RELAY_CONCURRENCY` |
| Request headers | 64 KiB | fixed |
| Ingress rate | 60 per minute per source (burst 30), 600 per minute per IP | fixed |
| Sign-in mail | 3 per hour per email, 10 per hour per IP | fixed |

A sweeper runs every minute and deletes deliveries that pass the retention age or exceed the per-source count. The relay counts an account's bytes on write. A full account gets `429` on ingress until something expires.

Unless an account turns on [encryption](#encryption), the relay stores requests as plaintext on disk. An over-cap `429` has no `Retry-After` header; a rate-limit `429` has one.

## Encryption

An account can have the relay seal its deliveries with [age](https://age-encryption.org) so the operator cannot read them. The relay holds only public keys.

- Each desktop registers an age public key when it connects. The user turns encryption on under **Settings ▸ Integrations ▸ Hive Relay**. See [Encryption](../inbox/sources.md#encryption) for what the user sees.
- Turning it on needs at least one registered key. Removing the last key while it is on is refused.
- The relay seals each later delivery to every key. Stored deliveries are not re-encrypted.
- A sealed account's `list_deliveries` returns metadata only.
- A sender can seal the body itself with `age -r <recipient>`. The desktop opens it with its key.

Scripts use `set_encryption`, `list_recipients`, `add_recipient`, and `remove_recipient` on the [MCP endpoint](#mcp-endpoint), or `PATCH /v1/me` and `/v1/recipients`.

## MCP endpoint

The relay serves its account API as MCP tools at `/mcp`, over stateless Streamable HTTP. Send an API key as `Authorization: Bearer <key>`. The tools act on the key's account.

| Tool | Does |
| --- | --- |
| `whoami` | Email, encryption setting, limits, usage, sign-in methods, version |
| `list_sources`, `create_source`, `update_source`, `delete_source` | Manage sources. `create_source` returns the ingress URL |
| `rotate_ingress` | Replace a source's ingress URL |
| `list_deliveries` | Page through stored deliveries. Metadata only on an encrypted account |
| `list_tokens`, `create_token`, `revoke_token` | Manage the account's API keys |
| `set_encryption`, `list_recipients`, `add_recipient`, `remove_recipient` | Manage [encryption](#encryption) |

`create_source` and `rotate_ingress` are the only places the ingress URL appears. `create_token` returns the key once. Revoking the key a call used ends that session's access.

Hive Desktop offers this server to agent workspaces. See [MCP servers](../chats/agent-workspaces.md#mcp-servers).

## Connect Hive Desktop

Open **Settings ▸ Integrations ▸ Hive Relay** and enter your relay's URL and sign in with a method the relay offers. See [Connect](../inbox/sources.md#connect). Then add a `sources.relay` node to a flow. See [Hive Relay](../inbox/sources.md#hive-relay) for the node's fields and the formats it recognises.
