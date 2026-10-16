#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$(dirname "$HIVE_CONFIG")" "$HIVE_DESKTOP_CONFIG_DIR" /home/hive/repos
if [[ ! -f "$HIVE_CONFIG" ]]; then
  cp /etc/hive-fixture.yaml "$HIVE_CONFIG"
fi
# Existing fixture volumes keep their configuration across image upgrades.
python3 - <<'PYCONFIG'
import os
from pathlib import Path
import yaml
path = Path(os.environ["HIVE_CONFIG"])
config = yaml.safe_load(path.read_text())
defaults = yaml.safe_load(Path("/etc/hive-fixture.yaml").read_text())
if "codex" not in config.setdefault("agents", {}):
    config["agents"]["codex"] = defaults["agents"]["codex"]
    path.write_text(yaml.safe_dump(config, sort_keys=False))
PYCONFIG
if [[ ! -d /home/hive/repos/demo/.git ]]; then
  git init -b main /home/hive/repos/demo
  git -C /home/hive/repos/demo config user.name 'Hive Remote Fixture'
  git -C /home/hive/repos/demo config user.email 'fixture@localhost'
  echo 'Persistent remote Hive test repository.' > /home/hive/repos/demo/README.md
  git -C /home/hive/repos/demo add README.md
  git -C /home/hive/repos/demo commit -m 'Seed remote fixture'
fi
if ! hive session list --json | python3 -c 'import json,sys; sys.exit(not any(json.loads(line)["name"] == "remote-demo" for line in sys.stdin))'; then
  hive session create --background --remote /home/hive/repos/demo remote-demo
fi
# A container restart loses tmux processes but retains Hive's session records.
hive session list --json | python3 -c 'import json,sys; [print(s["id"]) for line in sys.stdin if (s := json.loads(line))["name"] == "remote-demo" and s["state"] == "active"]' | while IFS= read -r id; do
  read -r slug directory < <(hive session show --json "$id" | python3 -c 'import json,sys; s=json.load(sys.stdin); print(s["slug"],s["path"])')
  if ! tmux has-session -t "=$slug" 2>/dev/null; then
    tmux new-session -d -s "$slug" -c "$directory"
    tmux new-window -t "=$slug" -n second -c "$directory"
  fi
done
mkdir -p "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-shell" "$HIVE_DESKTOP_CONFIG_DIR/flows"
if [[ ! -f "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-shell/agent-workspace.yaml" ]]; then
  cat > "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-shell/agent-workspace.yaml" <<'YAML'
version: 5
name: Remote test shell
command: bash --noprofile --norc -i
YAML
fi
mkdir -p "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-codex"
if [[ ! -f "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-codex/agent-workspace.yaml" ]]; then
  cat > "$HIVE_DESKTOP_CONFIG_DIR/workspaces/remote-codex/agent-workspace.yaml" <<'YAML'
version: 5
name: Remote Codex
command: codex --sandbox workspace-write --ask-for-approval on-request
YAML
fi
if [[ ! -f "$HIVE_DESKTOP_CONFIG_DIR/flows/remote-demo.yaml" ]]; then
  cat > "$HIVE_DESKTOP_CONFIG_DIR/flows/remote-demo.yaml" <<'YAML'
version: 1
name: Remote inbox
nodes:
  - { id: inbox, type: feed, name: Remote work }
wires: []
YAML
fi
exec hive-desktop
