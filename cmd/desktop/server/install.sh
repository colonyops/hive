#!/usr/bin/env bash
set -euo pipefail
[[ $(uname -s) == Linux ]] || { echo 'Install on Linux.' >&2; exit 1; }
[[ $EUID != 0 ]] || { echo 'Run as the SSH/agent account, without sudo.' >&2; exit 1; }
case "${1:-}" in ''|--no-start) ;; *) echo 'Usage: install.sh [--no-start]' >&2; exit 1 ;; esac
bundle=$(cd "$(dirname "$0")" && pwd)
case $(uname -m) in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; *) echo 'Unsupported architecture.' >&2; exit 1 ;; esac
[[ $(cat "$bundle/ARCH") == "$arch" ]] || { echo 'Download or build the bundle for this CPU.' >&2; exit 1; }
for command in git tmux systemctl; do command -v "$command" >/dev/null || { echo "Missing dependency: $command" >&2; exit 1; }; done
umask 077
prefix="$HOME/.local/share/hive-server"
config="$HOME/.config/hive-server"
units="$HOME/.config/systemd/user"
mkdir -p "$prefix/bin" "$prefix/data" "$prefix/engine" "$config/desktop" "$units"
chmod 700 "$prefix" "$config"
# Rename binaries so an upgrade does not overwrite a running executable.
for binary in hive hive-server; do
  install -m 755 "$bundle/bin/$binary" "$prefix/bin/$binary.next"
  mv -f "$prefix/bin/$binary.next" "$prefix/bin/$binary"
done
install -m 755 "$bundle/launch.sh" "$prefix/launch.sh"
install -m 644 "$bundle/"*.service "$units/"
install -m 644 "$bundle/REVISION" "$prefix/REVISION"
if [[ ! -f "$config/environment" ]]; then
  install -m 600 "$bundle/environment.example" "$config/environment"
fi
chmod 600 "$config/environment"
if [[ ! -f "$config/config.yaml" ]]; then
  printf 'version: "0.2.7"\ngit_path: git\n' > "$config/config.yaml"
fi
if [[ ${1:-} != --no-start ]]; then
  systemctl --user daemon-reload
  systemctl --user enable hive-server.service
  systemctl --user restart hive-server.service
fi
echo "Installed in $prefix"
echo "Credentials and overrides: $config/environment"
echo 'For startup at boot and operation after logout, run:'
printf '  sudo loginctl enable-linger %q\n' "$(id -un)"
echo 'Desktop connection file: ~/.local/share/hive-server/connection.json'
