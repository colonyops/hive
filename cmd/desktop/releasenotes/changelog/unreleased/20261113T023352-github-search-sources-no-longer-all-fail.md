---
kind: fixed
---

GitHub search sources no longer all fail when one search matches an item in an organization with SAML SSO enforcement that the token is not authorized for. Hive skips the items it cannot read and keeps the rest.
