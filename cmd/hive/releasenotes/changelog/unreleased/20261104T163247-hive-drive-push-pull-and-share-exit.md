---
kind: fixed
---

**`hive drive push`, `pull` and `share` exit 6 (unauthenticated) without a login, and an unknown flag exits 2 (invalid).** The commands used to report an internal error when no drive server was signed in or the server answered 401. The error text still says how to log in. A flag that a command does not define now follows the output rule: text on stderr, or one `{"kind", "message"}` JSON line when the command takes `--json` and you pass it.
