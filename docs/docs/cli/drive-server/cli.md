---
icon: lucide/cloud-upload
description: Sign in to a Hive Drive Server, then push, pull, and share documents with hive drive.
---

# Push, Pull, and Share

Four `hive drive` commands copy documents between a local [Hive Drive](../getting-started/context.md) space and a [drive server](deploy.md). Only markdown and html documents go up.

## Sign in

Create a token in the server's web UI, or ask the admin to run [`accounts token create`](policies.md#manage-accounts). Then:

```bash
hive drive login drive.example.com
```

The command asks for the token, checks it with the server, and stores it in the OS keychain under the server's host. It prints your email, policy, and storage use. Add `--token-stdin` to read the token from one line of stdin.

Hive Desktop uses the same keychain entry, so one sign-in serves both programs.

On a machine with no keychain, such as CI or a headless Linux host, skip `login` and set both variables:

```bash
export HIVE_DRIVE_SERVER=drive.example.com
export HIVE_DRIVE_TOKEN=hds_...
```

`HIVE_DRIVE_TOKEN` overrides any stored token.

A command picks the server in this order: `--server`, `HIVE_DRIVE_SERVER`, then the one server you are signed in to. With several logins and neither of the first two, the command asks for `--server`.

## Choose a space

Push, pull, and share work on one space. `hive drive` finds it from the current repository. Pass `--repo owner/repo`, `--workspace name`, or `--shared` before the subcommand to pick another:

```bash
hive drive --workspace hive push
hive drive -r colonyops/hive pull
```

## Push

```bash
hive drive push                  # every markdown and html document in the space
hive drive push plans/           # everything under a folder
hive drive push plans/ship.md --force
```

A document whose content matches its last upload is skipped. `--force` uploads it anyway. A failure on one document, such as a size or storage limit, does not stop the others. Bad credentials or no network stop the push. The exit status is non-zero when any path fails.

There is no remote delete in the CLI. A document you remove locally stays on the server until you delete it in the web UI.

## Pull

```bash
hive drive pull
hive drive pull plans/
```

Pull never overwrites or deletes a local file. A download that differs from a local document is written beside it as `<name>-1.md`, then `<name>-2.md`, using the first free number. A download that matches the local document, or one of its numbered copies, is reported as unchanged and writes nothing.

## Share

```bash
hive drive share plans/ship.md --expires 7d
```

This prints a public link. The document must already be on the server. See [Share links](sharing.md).

## Options

| Flag | Commands | Effect |
| --- | --- | --- |
| `--server` | push, pull, share | The server URL or host. |
| `--json` | push, pull, share | One JSON object per path, with `path`, `action`, and `error`. |
| `--force` | push | Upload documents that did not change. |
| `--expires` | share | `1d`, `7d`, or `30d`. |
| `--token-stdin` | login | Read the token from one line of stdin. |

## Upload markers

When a server is known, `hive drive ls` adds a column for upload state. `uploaded` means the file matches its last push. `changed` means it differs since then. A document never pushed has no marker. `ls` reads local records only and does not contact the server.
