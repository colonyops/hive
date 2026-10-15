# Kitty graphics render from the pane stream through a private OSC

- **Status:** accepted
- **Date:** 2026-10-15

## Context

Pi draws images with the Kitty graphics protocol: an APC (`ESC _ G … ESC \`)
with `C=1`, so the cursor stays put, and a size in cells. Pi prints the blank
rows itself. tmux does not implement Kitty graphics, but a control client
receives the pane's raw bytes in `%output`, the APC included. Like iTerm2's tmux
integration, the renderer can draw what tmux ignores. xterm.js 6.0 discards APC
and has no hook for it. Its image addon ties images to cells, which text
overwrites.

## Decision

A stream filter in front of `term.write` rewrites each `ESC _ G … ESC \` to
`ESC ] 71137 ; <nonce> ; … ESC \`, with a per-terminal random nonce so a
program's own OSC 71137 is ignored. An OSC handler receives it at its exact parser
position. The handler anchors each placement to a buffer line with a marker and
draws it on a canvas layer over the screen, never into cells.

The renderer never replies and never moves the cursor. tmux does not move its
cursor for an APC, and the rule of
[ADR the-embedded-emulator-never-answers-terminal-queries-on-a-tmux-pane](2026-08-08-the-embedded-emulator-never-answers-terminal-queries-on-a-tmux-pane.md)
holds. Hive Desktop sets `PI_IMAGE_PROTOCOL=kitty` only on the sessions,
windows and pop-ups it starts. Without it, Pi turns images off under `$TMUX`.

## Consequences

Images are live only. First paint, resync and reset rebuild from text. To get
them back, the control client marks a pane with `@hive-kitty-images` when its
output first carries a Kitty command; tmux keeps the mark across app restarts.
After each paint pass the client narrows every marked window by a column
through its per-window size for 150 ms, then clears the size. Pi redraws its
whole transcript on a width change, images included. Other clients attached
to the window see the resize too. tmux also treats the APC as a pane title,
so a Pi pane's title becomes the image payload until Pi sets it again.
Classification still resolves to `pi`, through the unanchored title pattern and
the process tier. Placements live on xterm markers, so an erase that disposes a line's marker
(`CSI J`, `CSI 1 J`, leaving the alternate screen) drops the image with it,
where Kitty would keep it. Programs that need replies or Unicode placeholders, such as
Claude Code or `kitten icat`, are not served.
