# Hive desktop shell

This directory is a Wails v3 Vue + TypeScript shell for Hive. It is deliberately
minimal while the desktop UI is developed in later phases.

## Native shell

On macOS, the shell uses `application.MacOptions.ActivationPolicy` set to
`application.ActivationPolicyRegular`, so Hive is a regular app with a Dock
icon. It also sets `ApplicationShouldTerminateAfterLastWindowClosed: false`,
keeping the application alive after its window closes.

The visible 1360×864 Hive window uses native hidden-inset titlebar chrome: a
`MacTitleBar` with `AppearsTransparent`, `HideTitle`, `FullSizeContent`,
`UseToolbar`, and `HideToolbarSeparator`, pinned to
`MacToolbarStyleUnifiedCompact` so AppKit cannot drift the toolbar height
across macOS versions, and `InvisibleTitleBarHeight: 42` so the traffic
lights center on the 42px HTML titlebar.

Closing the window hides it rather than destroying it, via a `WindowClosing`
hook registered with `RegisterHook`. It must be a hook, not `OnWindowEvent`:
hooks run synchronously before listeners, so `Cancel()` reliably aborts
Wails' internal window-destroy listener, which otherwise races the callback
in a separate goroutine. The app keeps running; reopen the window from the
Dock (`ApplicationShouldHandleReopen` calls `Show`) or from the tray menu.

The tray is a template icon with a menu: **Show Hive** calls `window.Show()`
and `window.Focus()`, and **Quit** calls `app.Quit()`. In the pinned Wails
alpha, `SystemTray.SetTemplateIcon` accepts exactly one `[]byte` PNG, so the
shell embeds only the retina `tray-templateTemplate@2x.png`; the 1x PNG is
still generated and committed as an asset but not embedded.

Manual Phase 2 verification remains required: check the Dock icon, native
traffic lights centered on the 42px titlebar, close-hides-window, reopening
from the Dock and from the tray menu, template-icon tinting in light and dark
menu bars, and Quit from the tray menu.

## Pinned versions

- Wails CLI and Go module: `github.com/wailsapp/wails/v3 v3.0.0-alpha2.116`
- npm runtime: `@wailsio/runtime 3.0.0-alpha.97`

`3.0.0-alpha.97` is the runtime version bundled by the pinned Wails Go module.
The `vue-ts` template alias was not available in alpha2.116; `wails3 init -t
vue -n hive-desktop` is that release's Vue + TypeScript template.

From the repository root, `mise install` provisions the matching Wails CLI.
The manual equivalent is:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.116
```

## Parent-module adaptations

The generated template normally has its own `go.mod`. Hive is a single Go
module, so `desktop/go.mod` and `desktop/go.sum` were removed. The desktop
entry point is the `github.com/colonyops/hive/desktop` package within the root
`github.com/colonyops/hive` module, and the Wails dependency is required by the
root `go.mod`.

The Wails Taskfiles still run from `desktop/`, which lets Go discover the
parent module automatically. Their `go:mod:tidy` task explicitly runs from the
module root, and their module-file task inputs point at `../go.mod` and
`../go.sum`. The Android and iOS binding-generation task inputs use those same
parent paths. The template's unused iOS option-overlay stubs were removed;
they were subdirectory files rather than code compiled with the desktop package.

A `main` package named `desktop` cannot use an unqualified `go build
./desktop` output at the repository root: Go would try to write a `desktop`
binary where this directory already exists. Always give the desktop binary an
explicit output path:

```sh
go build -o ./desktop/bin/hive-desktop ./desktop
go build -tags server -o ./desktop/bin/hive-desktop-server ./desktop
```

`frontend/dist/.gitkeep` is tracked and embedded by `//go:embed
all:frontend/dist`, so the first command also compiles before a frontend build.
`frontend/public/.gitkeep` is copied by Vite so the tracked dist placeholder is
restored after every frontend build. The rest of `frontend/dist` remains ignored.

Bindings must be generated while the current directory is `desktop/`, so the
Wails CLI identifies that directory as the application package while Go walks
up to the parent module:

```sh
cd desktop
wails3 generate bindings -clean=true -ts -i
```

The frontend Vite plugin requires generated typed-event bindings. The shell
registers the `feed:updated`, `auth:updated`, and `config:updated` events
using package-variable initialization rather than an `init()` function
because this repository enables `gochecknoinits` (main.go carries a comment
saying the same). All are wake-up signals: `auth:updated` makes the frontend
re-read auth Status (device-flow grants land in a Go goroutine),
`feed:updated` carries the profile ID whose data changed so the frontend
re-reads counts and, when it is the active profile, items, and
`config:updated` carries `"ok"` or the config error text after a
profiles-config reload.

The feed service delegates to a `feed.Provider`: mock fixtures in
`HIVE_DESKTOP_MOCK` modes, or the GitHub-backed `LiveProvider`. Live data is
acquired per **source** (a search query or the notifications inbox) and
cached by what is requested — kind + query + limit — so any number of feeds
and profiles reading the same source share one request. Feeds are client-side
filtered views over sources, deduplicated by `repo#num`, with app-local read
state under the hive data dir's `desktop/` subdirectory. In live mode a
poller refreshes the distinct source set across all profiles each 60s and
emits `feed:updated` per profile whose sources changed; the titlebar's
polling indicator reflects the active profile's unread count.

## Sources, profiles, and feeds as code

Sources, profiles ("workspaces"), and their feeds are defined in a
user-editable YAML file at `$XDG_CONFIG_HOME/hive/desktop/profiles.yaml`
(`~/.config` fallback; `HIVE_DESKTOP_CONFIG` overrides the path) —
deliberately in the config dir, not the data dir, so it can live in a
dotfiles repo. App-local state (read-state) stays in the data dir.

Sources acquire data from the GitHub API; feeds are client-side filtered
views over one or more sources. Only sources cost API requests — feeds are
unlimited and free.

```yaml
sources:
  - id: my-work               # unique across sources
    kind: search              # "search" | "notifications"
    query: "is:open involves:@me archived:false"   # search only
    limit: 50                 # optional; search: default 50 max 100,
                              # notifications: default 50 max 50 (API cap)
  - id: inbox
    kind: notifications
profiles:
  - id: triage                # stable slug; renaming makes it a new profile
    name: Triage
    feeds:
      - id: my-open-prs
        name: My open PRs
        sources: [my-work]    # at least one; ids must exist under sources
        filters:              # optional; groups AND, values OR, excludes win
          types: [pr]                     # pr | issue
          repos: ["colonyops/*"]          # owner/repo doublestar globs
          exclude_repos: ["colonyops/x"]
          authors: ["hay-kot"]            # case-insensitive; [ ] literal
          exclude_authors: ["*[bot]"]
          labels: ["bug", "area/*"]       # any item label matches any glob
          exclude_labels: ["wontfix"]
          reasons: [mention, review_requested]  # notification reasons
      - id: notifications-inbox
        name: Notifications inbox
        sources: [inbox]
```

Parsing is strict (unknown keys are errors; configs in the old
feed-level-kind/query schema fail with a hint) and validated: unique ids,
kind-specific query and limit rules, glob syntax, and the types/reasons
vocabularies. A `reasons` filter matches the notification reason, so items
known only from a search source never match it — reasons belong on feeds
that read a notifications source.

Rate-limit model: at most **25 search sources** — each distinct search
source is one request per poll (about once a minute) against GitHub's
search bucket of 30 requests/min, and 25 leaves headroom for manual
refreshes. Identical sources (same kind, query, and limit) deduplicate to
one request no matter how many feeds or profiles read them. Notifications
sources are uncapped: they poll the core bucket (5000/hr) with conditional
`If-Modified-Since` requests — an unchanged inbox answers 304 at no
rate-limit cost — and honor the server's `X-Poll-Interval` (min 60s) even
on manual refresh. There is no cap on feeds.

A `ConfigWatcher` (fsnotify on the config's parent directory, debounced)
hot-reloads the file on external edits: the store re-parses (keeping the
last-good sources and profiles when the new content is broken), the provider
cache is invalidated, and `config:updated` wakes the frontend. App-side
writes — creating a profile, source, or feed, and editing a feed — go
through YAML node-tree surgery so hand-written comments survive (comments
attached to a replaced feed node itself are lost), and the resulting
document is validated before it is written: a config that fails validation
never reaches disk. The "Feeds as code" sheet (sidebar FEEDS `+`, or ⌘K → "Edit feeds
as code…") shows the file, its validity, and a **Copy prompt** button that
puts a schema-complete prompt on the clipboard for a coding agent to edit
the config on the user's behalf.

Desktop-only Go code lives under `internal/desktop/**`; the `desktop/`
package is thin Wails wiring. `internal/desktop/auth` implements GitHub
authentication behind the auth service: an OAuth device flow plus a
personal-access-token fallback, with tokens stored in the OS keychain
(`HIVE_GITHUB_TOKEN` is a read-only headless override). The device flow uses
the registered Hive Desktop OAuth app's public client ID by default;
`HIVE_GITHUB_CLIENT_ID` overrides it, e.g. to test another registration.
`internal/github` is the shared GitHub REST client (deliberately not under
`internal/desktop`).

`HIVE_DESKTOP_MOCK` selects deterministic offline backends: `feed` starts
authenticated, `onboarding` starts signed out with a fake device flow that
grants after ~1.5s. Unset, the live backends run.

`build/config.yml` keeps `dev_mode.root_path: .`; when `wails3 dev` is started
from `desktop/`, Wails watches `desktop/` rather than the whole repository.

## Development and builds

Use the root mise tasks as the canonical entry points:

```sh
mise run desktop:generate # Regenerate frontend TS bindings.
mise run desktop:icons    # Regenerate committed icon assets.
mise run desktop:build    # Build the desktop app; on macOS emits desktop/bin/hive-desktop.
mise run desktop:serve    # Build and run the headless server build.
mise run desktop:dev      # Start Wails development mode.
```

`desktop:dev` runs `wails3 dev -config ./build/config.yml` from the desktop
application directory. The equivalent Taskfile command is `wails3 task dev`.

The alpha supports server builds. `desktop:serve` builds the frontend, then
compiles the pure HTTP-server variant without GUI dependencies to
`desktop/bin/hive-desktop-server` and runs it. The assets are `//go:embed`ded,
so frontend edits require re-running the task; the fast frontend loop is
`desktop:dev` with Vite HMR. The server defaults to `localhost:8080`; if that
port is taken, override it with the Wails-native `WAILS_SERVER_PORT` env var
(e.g. `WAILS_SERVER_PORT=9000 mise run desktop:serve`).

## Icons

The desktop icon masters live in `build/icons/`: `hive-mark.svg` is the
1024px amber Hive mark on its dark rounded-square field, and
`tray-template.svg` is the separate black-only 18px macOS template mark.
Regenerate every committed desktop icon with `mise run desktop:icons`.

The script requires librsvg (`rsvg-convert`), ImageMagick (`magick`), and
macOS `iconutil`; install the first two with `brew install librsvg imagemagick`.
The authoring toolchain was `rsvg-convert 2.62.3` and ImageMagick
`7.1.2-27`; inspect local versions with `rsvg-convert --version` and
`magick --version`. It strips volatile PNG metadata. Output is byte-stable when
regenerated on this authoring toolchain, but that guarantee does not extend to
other renderer or ImageMagick versions.

`build/darwin/icons.icns` is copied directly into macOS bundles; the Wails
scaffold's `appicon.icon` and `Assets.car` inputs were removed, so the template
icon generator is not part of any desktop build. `CFBundleIconFile` continues
to point at `icons.icns`. The Linux AppImage consumes the 512px
`build/appicon.png`; nFPM consumes the generated
`build/linux/icon-128.png` for its 128px hicolor installation path. The build
targets are macOS and Linux only, so no Windows assets are generated. The
generated `tray-templateTemplate.png` and `tray-templateTemplate@2x.png`
retain the macOS `Template` suffix for automatic tinting.

## Agent-driven UI verification

Use the headless server build for the UI verification loop:

```sh
mise run desktop:serve
```

Drive and inspect the app at `http://localhost:8080` with Playwright or browser
tooling, read the screenshots in `desktop/e2e/screenshots`, edit, and repeat.
Set `HIVE_DESKTOP_MOCK=onboarding` to drive the first-run screen offline.
Run `mise run desktop:e2e` as the regression gate. Its harness
(`desktop/e2e/scripts/serve.sh` and `playwright.config.ts`) deliberately runs
its own fresh build on port 8931 (in `feed` mock mode), so the gate never
reuses a stale interactive server, plus two `onboarding`-mode instances on
8932/8933 — one per browser project, because the mock auth backend stays
authenticated once its fake device flow grants. Before the first local run,
install the browsers with:

```sh
cd desktop/e2e
npx playwright install chromium webkit
```

Native shell behavior — the tray, the Dock, and close-hides-window — remains
a manual verification concern.
