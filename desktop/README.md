# Hive desktop shell

This directory is a Wails v3 Vue + TypeScript shell for Hive. It is deliberately
minimal while the desktop UI is developed in later phases.

## Pinned versions

- Wails CLI and Go module: `github.com/wailsapp/wails/v3 v3.0.0-alpha2.116`
- npm runtime: `@wailsio/runtime 3.0.0-alpha.97`

`3.0.0-alpha.97` is the runtime version bundled by the pinned Wails Go module.
The `vue-ts` template alias was not available in alpha2.116; `wails3 init -t
vue -n hive-desktop` is that release's Vue + TypeScript template.

Install the matching CLI with:

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

```sh
# Install frontend dependencies and build the embedded assets.
cd desktop/frontend
npm install
npm run build

# From the repository root, compile the desktop package.
go build -o ./desktop/bin/hive-desktop ./desktop

# Or start Wails development mode from the desktop application directory.
cd desktop
wails3 dev -config ./build/config.yml
# Equivalent Taskfile command: wails3 task dev
```

The alpha supports server builds. The following compiles the pure HTTP-server
variant without GUI dependencies:

```sh
go build -tags server -o ./desktop/bin/hive-desktop-server ./desktop
```

The server defaults to `localhost:8080` unless `application.Options.Server`
configures another host or port.

No custom icon assets were added in this phase; the scaffold retains the
Wails template's default build assets.
