#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$(dirname "$HIVE_CONFIG")" "$HIVE_DESKTOP_CONFIG_DIR" /home/hive/repos
if [[ ! -f "$HIVE_CONFIG" ]]; then
  cp /etc/hive-fixture.yaml "$HIVE_CONFIG"
fi
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
exec hive-desktop
