# Hive desktop shell

This directory is a Wails v3 Vue + TypeScript shell for Hive. It is deliberately
minimal while the desktop UI is developed in later phases.

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
registers the future-facing `feed:updated` event solely to generate those
bindings. It uses package-variable initialization rather than an `init()`
function, because this repository enables `gochecknoinits`. `FeedService` is
otherwise an empty registered placeholder with only `NewFeedService()`.

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

The alpha supports server builds. `desktop:serve` compiles the pure HTTP-server
variant without GUI dependencies to `desktop/bin/hive-desktop-server` and runs
it. The server defaults to `localhost:8080` unless `application.Options.Server`
configures another host or port.

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
`build/linux/icon-128.png` for its 128px hicolor installation path. Windows
uses the generated multi-resolution `build/windows/icon.ico`. The generated
`tray-templateTemplate.png` and `tray-templateTemplate@2x.png` retain the
macOS `Template` suffix for automatic tinting.
