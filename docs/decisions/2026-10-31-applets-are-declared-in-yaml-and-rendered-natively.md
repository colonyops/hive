# Applets are declared in YAML and rendered natively

- **Status:** accepted
- **Date:** 2026-10-31

## Context

A user should be able to ask an agent for a UI, from a Docker viewer or a picker over
mise tasks to something with no place in Hive's design, such as a game, and open it
from the app. These applets are written by or for the person whose machine they run
on, so restricting what they can do protects nobody and costs most of the value.

## Decision

An applet is a directory, `<ConfigDir>/applets/<id>/`, holding an `applet.yaml` that
picks one of two shapes.

**Declared** (`table`, `picker`): a data command, columns and actions, drawn with
Hive's components. This is the short path for lists. The frontend invokes commands by
id. A row reaches a command through `HIVE_ROW` and `HIVE_ROW_<KEY>` in the environment
rather than a template, because rows are command output and one missed `shq` in a
generated file would be shell injection by accident.

**Page** (`html`): the applet's own web page, served from its directory through the
app's asset handler, with `window.hive` injected before its scripts. `hive` runs any
command and reads or writes any file. There is no allowlist and no permission prompt.

The frame is sandboxed with an opaque origin. This is not a security boundary, since
the bridge can run anything. It keeps a page's mistakes off the app: a stray
`localStorage.clear()` or a global stylesheet cannot damage Hive. `localStorage` is
shimmed and persisted per applet, because generated pages expect it.

Nothing runs when a file loads. Commands start when the user opens an applet or acts
in it. Serving through the asset handler rather than the loopback server means nothing
outside this webview can read an applet's files or reach the bridge.

## Consequences

- An applet has the user's full access, and so does any remote script its page loads.
  Opening an applet from someone else is running their code.
- Cookies and IndexedDB do not work in an opaque origin.
- Keys in a frame do not reach the app's keymap, so the injected script forwards
  Escape, and a page keeps it by calling `preventDefault`.
- A declared applet cannot run on a server session, and a page falls back to its own
  directory there: commands run on this machine, as a pop-up terminal's do.
- A background action's output appears only in a toast. It is not an action run.
