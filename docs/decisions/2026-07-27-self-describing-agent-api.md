# Self-describing agent API: route index and generated OpenAPI

- **Status:** superseded by [mcp-replaces-the-agent-facing-http-api](2026-08-05-mcp-replaces-the-agent-facing-http-api.md)
- **Date:** 2026-07-27

> **Superseded (2026-08-05):** the route index and the generated OpenAPI
> document are deleted along with the surface they described (ADR mcp-replaces-the-agent-facing-http-api). The
> requirement they existed to meet — *an agent must be able to ask the running
> server what it serves, and the answer must be generated from the server's own
> types so it cannot drift* — is unchanged and is met by MCP's `tools/list`,
> whose input schemas are inferred from the same Go structs the handlers take.
> The operations table survives as the mux's source for the terminal control
> planes, but it no longer backs a generated document.

## Context

The agent HTTP API (ADR agent-http-api) had no way to describe itself. A consumer — the
intended one being an agent with no knowledge of the codebase — could not ask
the running server what it serves: `GET /` and `GET /openapi.json` both 404'd.
Enumerating the routes meant running `strings` over the shipped binary, and the
request shapes were lore: `PUT /api/profiles/{id}/image` takes a raw body, but
nothing said so, so a multipart-upload first guess failed with a misleading
"unsupported image" error (issue #97).

The API is loopback and agent-facing, so discovery should be an API call, not a
doc file or a config-chasing exercise. The repo already reflects Go structs into
JSON Schema with `invopop/jsonschema` for connector configs, and that same
reflection is slated to feed the MCP tool input schema and editor forms — the
"single declaration, many consumers" pattern (ADR go-owned-llm-prompts). Documentation should
extend that pattern, not introduce a parallel one.

## Decision

1. **One operations table is the single source.** `Controller.operations`
   declares each route — method, path, summary, query/request/response shapes,
   handler. `Handler` registers the mux from it, and the same table backs
   `GET /api` (a compact route index) and `GET /api/openapi.json` (the full
   spec). A handler with no table row is not served at all, so it is unreachable
   and `check:deadcode` fails — the table cannot silently omit a route.

2. **OpenAPI is generated, never authored.** Body schemas are reflected from the
   request/response structs with the *same* `invopop/jsonschema` reflector the
   connectors use — not annotations, and not a second reflector. OpenAPI 3.2's
   Schema Object is JSON Schema 2020-12, exactly what the reflector emits, so the
   schemas embed unchanged and the identical reflection will produce the MCP
   tool input schema later.

3. **Raw-binary bodies are declared, not implicit.** The avatar endpoints carry
   a `RawBinary` marker naming their media types and the "raw request body, not
   multipart/form-data" contract, so that requirement appears in both the index
   and the spec — the discoverability half of issue #97.

4. **The spec is validated in a test.** The document declares OpenAPI 3.2.0 —
   the newest its structure and the validator support — and a test validates it
   against the embedded 3.2 schema (`pb33f/libopenapi-validator`, test-only), so
   a struct that reflects into a malformed schema fails CI rather than a
   downstream consumer.

5. **The served document is self-targeting.** `GET /api/openapi.json` sets
   `servers` to the address the request arrived on, so the fetched spec points at
   the right loopback port without the caller editing it.

## Consequences

- Discovery is one request. No `strings` over the binary, no reverse-engineering
  the route set, no reading the request shapes from source.
- Adding a route is adding a table row; registration, the index, and the spec
  move together, and a handler left off the table fails `check:deadcode`.
- This is the first concrete step of the `app/command` / `app/query` enumeration
  the architecture's open question anticipates: the future MCP adapter ranges the
  same table for its tool list, from the same reflected schemas.
- A test-only dependency on `libopenapi`/`libopenapi-validator` enters the module
  graph; it is imported only from `_test.go` files, so it does not ship in the
  app binary.
- Out of scope here: base-URL discovery (issue #97 point 1) and rewording the
  handler's multipart error string. The spec documents the raw-body contract; the
  error-message wording and a stable base-URL file are separate follow-ups.

## Alternatives considered

- **A code-first OpenAPI library (kin-openapi, swaggest, huma).** Rejected: each
  ships its own schema reflector, which would fork the repo's invopop-based
  schema story — connector configs today, MCP tool schemas next — into a second
  dialect (kin-openapi models OpenAPI 3.0, not JSON Schema 2020-12), to replace a
  small serializer over a library already vendored. huma and fuego additionally
  require rewriting handlers off the ADR http-handler-conventions errchain shape.
- **Annotation generation (swaggo).** Rejected: it re-declares method, path, and
  params as comment annotations — a second source of truth beside the table, and
  the comment density the repo's style forbids.
- **A hand-written `openapi.json`.** Rejected: a second artifact to keep in sync,
  drift-gated forever, with no shared substrate for the MCP surface.
