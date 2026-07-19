# GitHub filter

A **GitHub filter** node narrows a stream of GitHub items down to the ones you care about — a faithful port of the feed filtering the app has always had, now usable anywhere in a flow. It has 1 input and **2 outputs**: port 0 (pass) and port 1 (fail).

## Fields

- `repos` / `exclude_repos` — one doublestar glob per line, matched against `owner/repo`.
- `authors` / `exclude_authors` — one glob per line, matched case-insensitively.
- `labels` / `exclude_labels` — one glob per line, matched against any of the item's labels.
- `types` — `pr` and/or `issue`.
- `reasons` — GitHub notification reasons (e.g. `mention`, `review_requested`). Items with no reason (search-only items) never match a reasons filter.

## Behavior

Groups AND together; values within a group OR; exclude groups win over includes. Leave port 1 unwired to get today's plain "drop on fail" behavior, or wire it up to route rejected items somewhere else (e.g. a low-priority feed).
