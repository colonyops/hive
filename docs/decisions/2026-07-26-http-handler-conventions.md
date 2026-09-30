# HTTP handler conventions: errchain, extractors, criterio

- **Status:** accepted
- **Date:** 2026-07-26

## Context

The first two JSON APIs (the `httpapi` adapter, ADR agent-http-api, and the devserver
control API, ADR devserver-agent-control-api) grew three different handler styles: `httpapi` handlers
called a `writeError` helper per call site, devserver handlers picked statuses
inline, and each surface had its own body-decoding and validation code. Before
building more endpoints on either surface, the shape needed to be fixed once —
modeled on recipinned, whose httpkit-based structure is the reference.

## Decision

1. **Handlers return `error`** (`httpkit/errchain`). Every route is registered
   through `chain.ToHandlerFunc`, and one error middleware — `web/mid.Errors` —
   maps error types to responses. No handler writes an error status inline.
   The middleware's built-in cases: an unreadable body (`web.BadRequestError`)
   is 400, a validation failure (`criterio.FieldErrors`) is 422 with a
   `fields` map. API-specific vocabulary is injected as `mid.Mapper` functions:
   `httpapi` maps `app.Error` Kinds (preserving ADR agent-http-api's map-Kind-once rule);
   the devserver maps everything else to a 500 that keeps the cause on the
   wire, since it is local dev tooling.

2. **Input enters through `web/extractors`.** `Body[T]` decodes JSON (1 MiB
   cap, unknown fields rejected) and `Query[T]` decodes the query string by
   `schema` tags (gorilla/schema); both then call the struct's `Validate()`
   when it has one. Struct tags carry only wire mapping; rules live in code.

3. **Validation is criterio, not tags.** Request structs implement
   `Validate() error` with `criterio.ValidateStruct`/`Run`, matching the
   `Validate` convention the registry pattern already uses. Cross-field rules
   are plain boolean expressions (`criterio.When`, `SkipIf`), and everything
   funnels into `FieldErrors` → one 422 shape.

4. **Controllers split per resource.** A `Controller` struct per surface,
   handlers as methods in `ctrl_<resource>.go` files, all routes registered in
   one place behind the chain. Response bodies are named structs, not
   `map[string]any`.

5. **One error wire shape.** `web.ErrorBody` is `{kind, message, fields?}` on
   both APIs. `httpapi` keeps `app.Error`'s `{kind, message}` marshaling; the
   devserver's former `{error}` shape is gone.

6. **`internal/webtools` became `internal/web`** — the shared home for the
   wire shapes, the version/build handler, `mid/` (error + request-log
   middleware), and `extractors/`. It must not import `internal/app`: the
   devserver consumes it, and Kind mapping stays in the adapter that owns the
   vocabulary.

## Consequences

- Query parameters are typed and validated: a bad `?limit=` is now a 422
  instead of a silently applied default, and "profile is required when feed is
  set" is a criterio rule, not handler branching.
- Validation failures moved from 400 to 422 on both APIs; 400 now means the
  body itself was unreadable. The devserver skill documents the new shape.
- The stdlib `ServeMux` stays; errchain composes with method patterns, so no
  router dependency was added. New deps: `httpkit`, `criterio`,
  `gorilla/schema` — criterio and schema are stdlib-only.
- `/_ctl/help` was dropped rather than converted: a bespoke discovery endpoint
  the dashboard never used, superseded by `/_ctl/state`'s `actions` list.
- The webhook ingestion handler (`/hooks/…`) is deliberately untouched: it is
  a wire-protocol endpoint with its own contract (constant-time secret check,
  202, plain-text rejections), not a JSON API.

## Alternatives considered

- **go-playground/validator struct tags.** Rejected for criterio: rules as
  code tie into the existing `Validate()` convention, cross-field rules stay
  readable, and no reflection-based tag DSL enters the repo.
- **recipinned's generic `adapters.Query/Action/Command` handler factories.**
  Deferred: with ~10 endpoints, plain handlers calling extractors are clearer;
  revisit if CRUD-shaped endpoints multiply.
- **chi.** Unnecessary — the Go 1.22 ServeMux already routes by method
  pattern, and errchain is router-agnostic.
