---
kind: fixed
---

An error from a server that does not answer now names the server and gives ssh's reason, instead of `context deadline exceeded`. A healthy server on a slow link now gets 30 seconds to connect, and a risk check gets a minute, so a slow connection or a large repository no longer fails a command. Every `hive` command now prints its error to stderr, so stdout stays clean for `--json`.
