#!/usr/bin/env bash
#
# Hive desktop installer
#
#     curl -fsSL https://hivedesktop.com/install.sh | bash
#
# Drop the `| bash` to read this first.
#
# Detects your OS + CPU, pulls the channel's latest build, verifies its SHA-256
# against the published manifest, and installs the app. On Linux it also checks
# the runtime libraries and registers Hive with the desktop application menu.
#
# Channel defaults to stable. Track another by passing the flag through bash:
#
#     curl -fsSL <url> | bash -s -- --channel dev
#
# or set `HIVE_CHANNEL`.
#
set -euo pipefail

DL_BASE="${HIVE_DL_BASE:-https://dl.hivedesktop.com}"
CHANNEL="${HIVE_CHANNEL:-stable}"
APP_NAME="Hive.app"
APP_EXE="hive-desktop" # CFBundleExecutable inside Hive.app / linux binary name

if [ -t 1 ]; then
  BOLD=$'\033[1m'; DIM=$'\033[2m'; RED=$'\033[31m'; GRN=$'\033[32m'; RST=$'\033[0m'
else
  BOLD=; DIM=; RED=; GRN=; RST=
fi

say() { printf '%s %s\n' "${DIM}hive${RST}" "$*"; }
ok()  { printf '%s %s\n' "${GRN}✓${RST}" "$*"; }
die() { printf '%s %s\n' "${RED}✗${RST}" "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "this installer needs \`$1\` on your PATH"; }

# `bash -s -- --channel dev` passes the flag through to here; it wins over the
# HIVE_CHANNEL env var.
parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --channel) [ $# -ge 2 ] || die "--channel needs a value"; CHANNEL="$2"; shift 2 ;;
      --channel=*) CHANNEL="${1#*=}"; shift ;;
      -h | --help)
        printf 'usage: install.sh [--channel stable|beta|dev]\n'
        exit 0 ;;
      *) die "unknown option: $1 (try --help)" ;;
    esac
  done
}

detect_platform() {
  case "$(uname -s)" in
    Darwin) OS=darwin ;;
    Linux)  OS=linux ;;
    *) die "unsupported OS: $(uname -s). Hive ships for macOS and Linux" ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) ARCH=arm64 ;;
    x86_64 | amd64)  ARCH=amd64 ;;
    *) die "unsupported CPU: $(uname -m)" ;;
  esac
}

# Candidate manifest platform keys, best match first.
platform_keys() {
  if [ "$OS" = darwin ]; then
    printf '%s\n' "darwin-universal" "darwin-$ARCH"
  else
    printf '%s\n' "linux-$ARCH"
    [ "$ARCH" = amd64 ] && printf '%s\n' "linux-x86_64"
  fi
}

# Reads $MANIFEST (indented JSON) and prints one field of one platform block.
json_field() { # $1=platform-key  $2=field
  printf '%s\n' "$MANIFEST" | awk -F'"' -v key="$1" -v field="$2" '
    $2 == "platforms" { in_p = 1 }
    in_p && $2 == key { in_b = 1; next }
    in_b && $2 == field { print $4; exit }
    in_b && /}/ { in_b = 0 }'
}

resolve_from_channel() {
  local url="$DL_BASE/desktop/channels/$CHANNEL/latest.json"
  MANIFEST="$(curl -fsSL --proto '=https' --tlsv1.2 "$url")" \
    || die "couldn't fetch the $CHANNEL manifest ($url)"
  VERSION="$(printf '%s\n' "$MANIFEST" | awk -F'"' '/"version":/ { print $4; exit }')"
  local key
  # `url`, not `installer_url`: on macOS the manifest also advertises a .dmg,
  # but that exists to give a browser download a drag-to-Applications window.
  # This script already puts the app there, so the zip saves it a mount cycle.
  for key in $(platform_keys); do
    ART_URL="$(json_field "$key" url)"
    if [ -n "$ART_URL" ]; then
      ART_SHA="$(json_field "$key" sha256)"
      PLATFORM="$key"
      return
    fi
  done
  die "the $CHANNEL channel has no ${OS}-${ARCH} build yet. macOS is the only published platform"
}

verify_sha() {
  [ -n "$ART_SHA" ] || die "the manifest has no checksum for $PLATFORM"
  local actual
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$PKG" | awk '{ print $1 }')"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$PKG" | awk '{ print $1 }')"
  else
    die "need \`shasum\` or \`sha256sum\` to verify the download"
  fi
  [ "$actual" = "$ART_SHA" ] || die "checksum mismatch: expected $ART_SHA, got $actual"
}

install_darwin() {
  need unzip
  unzip -q "$PKG" -d "$WORK/x" || die "failed to unzip the download"
  [ -d "$WORK/x/$APP_NAME" ] || die "archive did not contain $APP_NAME"
  local dest="/Applications"
  [ -w "$dest" ] || { dest="$HOME/Applications"; mkdir -p "$dest"; }
  say "installing to ${BOLD}$dest/$APP_NAME${RST}…"
  rm -rf "${dest:?}/${APP_NAME:?}"
  if command -v ditto >/dev/null 2>&1; then
    ditto "$WORK/x/$APP_NAME" "$dest/$APP_NAME"
  else
    cp -R "$WORK/x/$APP_NAME" "$dest/$APP_NAME"
  fi
  APP_PATH="$dest/$APP_NAME"
  EXE_PATH="$APP_PATH/Contents/MacOS/$APP_EXE"
}

check_linux_runtime() {
  command -v ldd >/dev/null 2>&1 || return 0
  local output missing ldd_status=0
  output="$(LC_ALL=C ldd "$EXE_PATH" 2>&1)" || ldd_status=$?
  missing="$(printf '%s\n' "$output" | awk '$2 == "=>" && $3 == "not" && $4 == "found" { print $1 }')"
  if [ -z "$missing" ] && [ "$ldd_status" -eq 0 ]; then
    return
  fi

  if [ -n "$missing" ]; then
    printf '%s\n' "$missing" | while IFS= read -r library; do
      say "missing runtime library: ${BOLD}$library${RST}"
    done
  else
    say "couldn't verify the installed binary:"
    printf '%s\n' "$output" >&2
  fi
  say "Linux requirements: ${BOLD}https://hivedesktop.com/getting-started/troubleshooting/#hive-does-not-start-on-linux${RST}"
  die "Hive is installed but this system does not meet its Linux runtime requirements"
}

install_linux_desktop_entry() {
  local data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  local applications_dir="$data_home/applications"
  local icon_dir="$data_home/icons/hicolor/128x128/apps"
  local desktop_file="$applications_dir/hive-desktop.desktop"
  local icon_file="$icon_dir/hive-desktop.png"
  local escaped_exe="$EXE_PATH"
  escaped_exe="${escaped_exe//\\/\\\\}"
  escaped_exe="${escaped_exe//\"/\\\"}"
  escaped_exe="${escaped_exe//%/%%}"

  mkdir -p "$applications_dir" "$icon_dir"
  if ! curl -fsSL --proto '=https' --tlsv1.2 \
    -o "$icon_file" "https://hivedesktop.com/assets/hive-desktop.png"; then
    rm -f "$icon_file"
    say "couldn't download the application icon; installing the launcher without it"
  fi

  cat > "$desktop_file" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=Hive
Comment=Desktop workspace for feeds, code, and agent chats
Exec="$escaped_exe"
Icon=hive-desktop
Categories=Development;
Terminal=false
StartupNotify=true
StartupWMClass=hive-desktop
EOF
  chmod 0644 "$desktop_file"
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$applications_dir" >/dev/null 2>&1 || true
  fi
}

install_linux() {
  need tar
  local dir="${HIVE_HOME:-$HOME/.local/share/hive}"
  mkdir -p "$dir"
  dir="$(cd "$dir" && pwd -P)"
  tar -xzf "$PKG" -C "$dir" || die "failed to extract the download"
  EXE_PATH="$dir/$APP_EXE"
  [ -x "$EXE_PATH" ] || EXE_PATH="$(find "$dir" -maxdepth 2 -type f -name "$APP_EXE" | head -1)"
  [ -n "$EXE_PATH" ] || die "couldn't find the $APP_EXE binary in the archive"
  chmod +x "$EXE_PATH"
  APP_PATH="$dir"
  install_linux_desktop_entry
  check_linux_runtime
}

print_next() {
  echo
  ok "installed ${BOLD}Hive $VERSION${RST} → $APP_PATH"
  echo
  if [ "$OS" = darwin ]; then
    say "launch it: ${BOLD}open -a Hive${RST}"
  else
    say "launch it from your application menu, or run: ${BOLD}$EXE_PATH${RST}"
  fi
  say "next: connect GitHub, then build your first feed."
}

main() {
  parse_args "$@"
  need curl
  need awk
  detect_platform
  say "installing Hive for ${BOLD}${OS}-${ARCH}${RST}…"
  resolve_from_channel
  say "release ${BOLD}$VERSION${RST} · $PLATFORM · $CHANNEL"
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  PKG="$WORK/pkg"
  say "downloading…"
  curl -fSL --proto '=https' --tlsv1.2 -o "$PKG" "$ART_URL" || die "download failed: $ART_URL"
  verify_sha
  ok "checksum verified"
  if [ "$OS" = darwin ]; then install_darwin; else install_linux; fi
  print_next
}

main "$@"
