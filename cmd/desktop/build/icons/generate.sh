#!/usr/bin/env bash
# Render Hive's committed desktop icon assets from the SVG masters in this directory.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
build_dir=$(cd -- "$script_dir/.." && pwd)
mark="$script_dir/hive-mark.svg"
tray="$script_dir/tray-template.svg"
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/hive-icons.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT

require() {
  local command=$1
  local hint=$2
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'generate.sh: %s is required; %s\n' "$command" "$hint" >&2
    exit 1
  fi
}

require rsvg-convert 'install it with: brew install librsvg'
require magick 'install it with: brew install imagemagick'
require iconutil 'install Xcode Command Line Tools with: xcode-select --install'

render() {
  local svg=$1
  local size=$2
  local output=$3

  rsvg-convert --width "$size" --height "$size" "$svg" |
    magick png:- -strip -define png:exclude-chunks=date,time \
      -define png:compression-level=9 -define png:compression-filter=5 \
      -define png:color-type=6 "PNG32:$output"
}

mkdir -p "$build_dir/darwin" "$build_dir/linux"

# macOS requires these exact iconset names, including the @2x representations.
iconset="$tmpdir/hive.iconset"
mkdir -p "$iconset"
render "$mark" 16 "$iconset/icon_16x16.png"
render "$mark" 32 "$iconset/icon_16x16@2x.png"
render "$mark" 32 "$iconset/icon_32x32.png"
render "$mark" 64 "$iconset/icon_32x32@2x.png"
render "$mark" 128 "$iconset/icon_128x128.png"
render "$mark" 256 "$iconset/icon_128x128@2x.png"
render "$mark" 256 "$iconset/icon_256x256.png"
render "$mark" 512 "$iconset/icon_256x256@2x.png"
render "$mark" 512 "$iconset/icon_512x512.png"
render "$mark" 1024 "$iconset/icon_512x512@2x.png"
iconutil -c icns "$iconset" -o "$build_dir/darwin/icons.icns"

# appicon.png is used by Linux AppImage; the package icon below is its 128px peer.
render "$mark" 512 "$build_dir/appicon.png"
render "$mark" 128 "$build_dir/linux/icon-128.png"

# Template-suffixed tray assets let macOS tint the pure-black mark automatically.
render "$tray" 18 "$script_dir/tray-templateTemplate.png"
render "$tray" 36 "$script_dir/tray-templateTemplate@2x.png"

# Linux has no template-icon concept: wails' StatusNotifierItem backend pushes
# the bytes to the panel as a raw pixmap (systemtray_linux.go setIcon), so the
# pure-black macOS mark would render invisible on the dark panels GNOME and
# Ubuntu use unconditionally. Negate the RGB channels — alpha is untouched, so
# the masked prompt cutout survives — to get the white mark Linux panels expect.
# 44px is 2x the 22px panel icon size, so HiDPI panels downscale cleanly.
rsvg-convert --width 44 --height 44 "$tray" |
  magick png:- -channel RGB -negate +channel -strip \
    -define png:exclude-chunks=date,time \
    -define png:compression-level=9 -define png:compression-filter=5 \
    -define png:color-type=6 "PNG32:$build_dir/linux/tray-icon.png"
