---
kind: fixed
---

A session command against a server under `servers:` (experimental) no longer hangs on a stuck daemon: it fails after 10 seconds with no change, and a delete gives up after 2 minutes. A shell startup file on the server that prints text no longer breaks the connection with `http2: frame too large`. `hive hc list --session` warns about a `<server>:<id>` value, which matches no items.
