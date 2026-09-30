# Docs

The public site and the contributor docs for this repository. `docs/docs/` holds
the site's pages (the landing page, the CLI docs, and the Hive Desktop docs;
see [`AGENTS.md`](AGENTS.md)). Everything else in this directory is for
contributors.

## Decisions

Notable architecture/infrastructure decisions are recorded as ADRs in
[`decisions/`](decisions/), one file per decision — the directory listing is
the index.

An ADR is identified by its filename, `YYYY-MM-DD-slug.md`. Nothing allocates a
number, so two branches can add one without colliding, and prose cites the slug
alone: `(ADR terminal-transport)`. Start one with `mise run adr:new -- "Title"`;
`mise run check:adr` verifies the ids and the citations. Superseded
ADRs are marked, not deleted.

## References

- User-facing product docs are the site: source under [`docs/`](docs/), published at [hivedesktop.com](https://hivedesktop.com). [`distribution.md`](distribution.md) is the maintainer counterpart.
- [`architecture.md`](architecture.md) — how the app is structured and how it should grow: the core/adapter shape, named patterns, directory layout, extension points, cross-cutting conventions, and the rules PRs are reviewed against. Read this before adding a subsystem, entrypoint, or extension point.
- [`source-pipeline.md`](source-pipeline.md) — the pipeline's runtime behaviour: ingestion, the `Msg` contract, flows, membership replay, retention, and actions.
- [`distribution.md`](distribution.md) — concrete distribution infra: bucket, domains, bucket layout, manifest schema, publish/rollback runbook, credentials.
- [`development.md`](development.md) — repository layout, setup, running the app, and the quality gates.
