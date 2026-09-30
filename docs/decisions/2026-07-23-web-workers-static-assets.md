# Landing page as Cloudflare Workers static assets

- **Status:** accepted; the site behind the assets is amended by [ADR the-site-is-zensical-built-under-mise-served-as-worker-static-assets](2026-09-07-the-site-is-zensical-built-under-mise-served-as-worker-static-assets.md) — the hosting decision here still holds
- **Date:** 2026-07-23

## Context

The landing page needed hosting with a custom domain and CI deploys. Cloudflare Pages was the original plan, but Pages and Workers have converged: Cloudflare's guidance for new static sites is Workers static assets, Pages is feature-frozen, and the `cf` CLI does not expose Pages at all.

## Decision

`web/` is a Workers static-assets project: `web/public/` holds the site, `web/wrangler.jsonc` declares the worker (`hive-desktop-web`) and the custom domain `hivedesktop.com` with `custom_domain: true` — the domain, DNS record, and certificate attach on deploy with no dashboard steps. Deploys run `npm ci && npm run deploy` (lockfile-pinned wrangler, no third-party actions) via `.github/workflows/deploy-web.yml` on pushes to main touching `web/**`.

## Consequences

- Same outcome as Pages (edge-served static site, custom domain, CI deploys) with config-driven domain wiring.
- When the download link needs a redirect endpoint reading `latest.json`, it is a few lines of worker code in front of the same assets — no separate Functions product.
- CI requires a `CLOUDFLARE_API_TOKEN` repo secret (Workers edit scope on the account, zone scope for `hivedesktop.com`).
