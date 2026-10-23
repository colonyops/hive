# HTTP handler conventions: errchain, extractors, criterio

- **Status:** accepted
- **Date:** 2026-07-26

## Context

The JSON APIs in this repo (the desktop's `httpapi` adapter, the devserver control API, the drive
server's `httpapi`) need one handler shape, so each surface does not grow its own error writing, body
decoding, and validation.

## Decision

1. **Handlers return `error`** (`httpkit/errchain`). Routes register through `chain.ToHandlerFunc`, and one
   middleware, `internal/web/mid.Errors`, maps errors to responses. No handler writes an error status
   inline. Built-in cases: an unreadable body (`web.BadRequestError`) is 400, an oversized body is 413, a
   validation failure (`criterio.FieldErrors`) is 422 with a `fields` map. Each API injects its own
   vocabulary as a `mid.Mapper` (the desktop `httpapi` maps `app.Error` kinds in `mapAppError`; the
   devserver maps everything else to a 500 that keeps the cause on the wire).
2. **Input enters through `internal/web/extractors`.** `Body[T]` decodes JSON (1 MiB cap, unknown fields
   rejected) and then calls the struct's `Validate()` when it has one. Struct tags carry only wire mapping.
3. **Validation is criterio, not tags.** Request structs implement `Validate() error` with criterio, the
   same `Validate` convention the registries use.
4. **Controllers split per resource**: handlers as methods in `ctrl_<resource>.go`, routes registered in
   one place. Response bodies are named structs, not `map[string]any`.
5. **One error wire shape**: `web.ErrorBody` is `{kind, message, fields?}`.
6. **`internal/web`** holds the wire shapes, the version handler, `mid/`, and `extractors/`. It imports no
   program code; kind mapping stays in the adapter that owns the vocabulary.

## Consequences

- 400 means the body was unreadable; 422 means it decoded but failed validation.
- The stdlib `ServeMux` stays; errchain composes with method patterns, so there is no router dependency.
- The webhook ingestion handler (`/hooks/...`) is not a JSON API and does not follow these conventions: it
  has its own wire contract (constant-time secret check, 202, plain-text rejections).
