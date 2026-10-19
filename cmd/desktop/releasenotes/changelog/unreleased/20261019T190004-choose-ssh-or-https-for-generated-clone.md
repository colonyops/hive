---
kind: added
---

**Choose SSH or HTTPS for generated clone URLs** in Settings → General or with `git.clone_protocol` in `settings.yaml`. HTTPS remains the default. Set an existing action’s repository template to `{{ .ItemRemote }}` to follow the preference. Explicit repository URLs, including `git://`, keep their protocol. The unencrypted `git://` transport is not available as a global preference.
