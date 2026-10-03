---
kind: changed
---

Session names have one set of rules. A name is unique on its slug, so `Fix Bug` and `fix-bug` count as the same session, and a name is at most 60 characters. `hive batch` now checks names the same way as `hive new`. A source template whose name is taken gets the item id as a suffix instead of failing, and accented letters in a title fold to their base letter (`Café` to `cafe`).
