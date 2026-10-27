---
icon: lucide/shield-check
description: Choose who can sign in to a Hive Drive Server and their storage limit, and manage accounts and tokens.
---

# User Policies

`user_policies` in `drive-server.yaml` is the only way to admit people. A policy names the email addresses it covers and sets a storage limit for them.

```yaml
user_policies:
  - name: team
    matches:
      - "*@example.com"
    quotas:
      limit: 5GiB
  - name: guests
    matches:
      - "ana@example.org"
      - "*@partner.example"
    sign_up: false
    quotas:
      limit: 500MB
```

| Key | Meaning |
| --- | --- |
| `name` | Required. Unique. `accounts list` shows it. |
| `matches` | Required. Email globs. |
| `sign_up` | Whether a new account can be created through this policy. Default `true`. |
| `quotas.limit` | Required. The storage limit per account. |

## Matching

The first policy with a glob that matches the email applies. Order policies from the most specific to the most general. Matching ignores case.

Quote every glob. An unquoted `*` starts a YAML alias and the file fails to parse.

A policy below a catch-all `"*"` never matches. The server logs a warning for it.

An email that matches no policy cannot sign in and cannot use its tokens.

To treat one person differently, add a policy with their exact address above the broader ones. It can set another limit or `sign_up: false`.

## Sign-up

With `sign_up: false`, the policy admits only emails that already have an account. The sign-in page gives the same answer for every address, so it does not show which emails exist. To create an account for such an email, run `accounts token create`, which makes the account when a policy matches.

## Storage limit

`quotas.limit` is `unlimited` or a positive size. `0` and negative values are invalid, so a limit is never implied by accident. Decimal units (`KB`, `MB`, `GB`, `TB`) are powers of 1000 and binary units (`KiB`, `MiB`, `GiB`, `TiB`) are powers of 1024. `5GB` is 5,000,000,000 bytes and `5GiB` is 5,368,709,120 bytes. A bare number is bytes.

A push that would pass the limit fails for that document.

## Locked accounts

When no policy matches an account's email any more, the account is locked. Its data stays, but its sessions and tokens stop working at once. Restore a matching policy to unlock it.

## Manage accounts

The `accounts` commands open the databases directly. Run them where the server's data and config directories are visible, for example with `docker exec`. They accept `--config-dir`, `--data-dir`, and `--base-url` like `serve`.

```bash
docker exec drive-server /hive-drive-server accounts list
docker exec drive-server /hive-drive-server accounts delete ana@example.org --yes
docker exec drive-server /hive-drive-server accounts token create ana@example.org --scope push --expires 30d
```

- `accounts list` prints each email, ID, creation date, and the policy that admits it. A locked account shows `locked`.
- `accounts delete <email> --yes` removes the account, its tokens, and its sessions. It works on locked accounts. Without `--yes` it does nothing.
- `accounts token create <email> --scope read|push` prints a token once. `--name` labels it (default `cli`). `--expires` takes `30d` or `720h`, and without it the token never expires. The command creates the account when a policy matches the email.

A `read` token lists and gets documents. A `push` token also writes, deletes, and shares. People can also create tokens in the web UI after they sign in. Use a token with [`hive drive login`](cli.md#sign-in).

## Changing policies

The server reads `drive-server.yaml` at startup. Restart it after you change `user_policies` or any other key.
