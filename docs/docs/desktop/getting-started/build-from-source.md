---
icon: lucide/hammer
description: Build Hive Desktop from a repository clone on macOS or Linux.
---

# Build from source

Use the repository's `CONTRIBUTING.md` and `docs/development.md` if you plan to change the code. This page covers a local build.

## Requirements

- Git
- [mise](https://mise.jdx.dev/)
- Node.js 22 and npm
- Docker for Linux builds
- tmux 3.2 or newer for Code and Chats

Windows is not supported.

## Set up

```sh
git clone https://github.com/hay-kot/hive-desktop.git
cd hive-desktop
mise trust
mise install
cd desktop/frontend && npm ci && cd ../..
```

## Build

```sh
mise run build
```

The binary is written to `desktop/bin/hive-desktop`. On macOS, create a local app bundle with:

```sh
cd desktop && wails3 package
```

For Linux, Docker must be running:

```sh
mise run build:linux
ARCH=arm64 mise run build:linux
```

## Run a development build

```sh
mise run dev
```

Development runs use isolated config and data directories. See `docs/development.md` in the repository for the devserver, fixtures, tests, and quality checks.
