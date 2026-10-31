# The Changes view renders through @pierre/diffs

- **Status:** accepted
- **Date:** 2026-10-30

## Context

The Changes view is one scroll over every diff in a session, up to 1,000 files.
Its first renderer mounted every row of each file near the viewport and
highlighted each hunk on the main thread. On a 150-file, 45,000-line change it
held 53,000 DOM nodes and blocked the main thread for over 4 seconds during a
fast scroll. Dropping rows per file could not fix it: one large file was still
thousands of rows, and one large hunk was still one long highlight job.

The view will also grow line comments, which need line selection and rows
injected between lines. A diff renderer has to own the layout to do that without
giving up virtualization.

## Decision

1. **`@pierre/diffs` `CodeView` renders the stream, through its vanilla API.** It
   mounts only the lines near the viewport. `lib/diffItems.ts` turns the session
   API's patch text into its items, and `DiffStream.vue` owns the viewer. Comments
   build on its annotation and selection APIs. Plannotator's review UI uses the same
   library for the same feature.

2. **The version is pinned exactly.** The worker pool is marked experimental and
   `unsafeCSS` has no compatibility promise, even across patch versions. A bump
   needs the e2e spec and a look at the rendered view.

3. **Highlighting runs in a pool of three workers, with `shiki-wasm`.** Oniguruma for
   the reason `lib/highlight.ts` gives. Hive's code themes are TextMate themes, so
   the faster `highlights` backend is out: it would drop the app's syntax colors.
   The worker is the portable build, loaded by URL, since the bundled entry ships a
   second copy of every grammar.

4. **Every row has a fixed height.** Lines scroll instead of wrapping, and
   `DIFF_METRICS` holds the sizes CodeView estimates from. That makes off-screen
   layout exact, so the scrollbar holds still and hunk jumps can be computed.

5. **File headers stay Vue components,** teleported into the light-DOM slot CodeView
   gives custom headers. A file with no text to draw is collapsed and says why on
   its header row, so it adds no row of a different height.

6. **Diffs are read ahead of the viewport,** one batch at a time, outward from the
   file on screen, up to the 200 open files nearest it. A file joins the stream only once its diff
   is read, because growing it in place would push everything below it down the
   screen. Reading only what is on screen would leave a scrollbar too short to use.

## Consequences

- Every language shiki knows highlights. That puts about 13 MB of grammars and themes
  in the app's assets, loaded only when a file needs them.
- The diff DOM sits in shadow roots. Tests reach it through Playwright, which pierces
  open roots; Vitest fakes `CodeView`, because jsdom has no layout.
- Split view no longer wraps long lines.
- The per-file line cap rose from 1,500 to 20,000. `maxDiffBytes` is what bounds a read.
