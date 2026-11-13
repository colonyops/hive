#!/usr/bin/env bash
# Builds libghostty (the embedding library Ghostty's macOS app links) at a
# pinned commit into cmd/desktop/build/ghostty/, where the `ghostty` build tag
# of the nativeterm adapter links it. Needs Zig (the version Ghostty's
# build.zig.zon asks for) and Xcode with the Metal toolchain
# (`xcodebuild -downloadComponent MetalToolchain`).
set -euo pipefail

GHOSTTY_COMMIT="${GHOSTTY_COMMIT:-cf3a8a6594eec899fe6335465668c60f2281ce9c}"
ZIG_VERSION="${ZIG_VERSION:-0.17.1}"

root="$(cd "$(dirname "$0")/../.." && pwd)"
out="$root/build/ghostty"
src="$out/src"

if [[ "$(uname -s)" != Darwin ]]; then
	echo "libghostty's embedding library is built for macOS only" >&2
	exit 1
fi

if [[ -f "$out/lib/libghostty-internal.a" && "$(cat "$out/COMMIT" 2>/dev/null)" == "$GHOSTTY_COMMIT" ]]; then
	echo "libghostty $GHOSTTY_COMMIT is already built in $out"
	exit 0
fi

if [[ ! -d "$src/.git" ]]; then
	git init -q "$src"
	git -C "$src" remote add origin https://github.com/ghostty-org/ghostty
fi
git -C "$src" fetch -q --depth 1 origin "$GHOSTTY_COMMIT"
git -C "$src" checkout -q --force FETCH_HEAD

zig="${ZIG:-$(mise where "zig@$ZIG_VERSION" 2>/dev/null)/zig}"
if [[ ! -x "$zig" ]]; then
	mise install "zig@$ZIG_VERSION"
	zig="$(mise where "zig@$ZIG_VERSION")/zig"
fi

(cd "$src" && "$zig" build \
	-Dapp-runtime=none \
	-Demit-xcframework=true \
	-Demit-macos-app=false \
	-Dxcframework-target=native \
	-Doptimize=ReleaseFast)

slice="$(find "$src/macos/GhosttyKit.xcframework" -maxdepth 1 -type d -name 'macos-*' | head -1)"
rm -rf "$out/lib" "$out/include"
mkdir -p "$out/lib" "$out/include"
cp "$slice/libghostty-internal.a" "$out/lib/"
cp "$slice/Headers/ghostty.h" "$out/include/"
echo "$GHOSTTY_COMMIT" >"$out/COMMIT"
echo "libghostty built into $out"
