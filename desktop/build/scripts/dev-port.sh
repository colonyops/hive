#!/usr/bin/env sh
# Prints the TCP port the Vite dev server should bind for `wails3 dev`.
#
# Honors an explicit WAILS_VITE_PORT (so a port can be pinned); otherwise asks
# the OS for a free ephemeral port. This is what lets multiple `hive` desktop
# dev instances run at once instead of colliding on the default 9245 — `wails3
# dev` prechecks the port with net.Listen and hard-errors if it is already in
# use. node is always present here (the frontend build depends on it).
if [ -n "${WAILS_VITE_PORT}" ]; then
  printf '%s\n' "${WAILS_VITE_PORT}"
else
  node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{const p=s.address().port;s.close(()=>console.log(p))})'
fi
