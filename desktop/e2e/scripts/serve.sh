#!/usr/bin/env bash
set -euo pipefail

cd ../../
(
  cd desktop/frontend
  npm run build
)
mkdir -p desktop/bin

# The feed-mode server's flows/*.yaml directory: a checked-in fixture (one
# flow, whose feed node id matches desktop/mockseed.go's seeded feed_item
# rows) rather than desktop.FlowsDir()'s default — keeps the mock sidebar's
# profile/feed list deterministic instead of depending on whatever XDG
# config happens to exist on the machine running the gate.
FEED_FLOWS_DIR="$(pwd)/desktop/e2e/fixtures/flows"
FEED_ACTIONS_PATH="$(pwd)/desktop/e2e/fixtures/actions.yml"
# CGO_ENABLED=0: the server-mode binary is pure Go (matches the Wails server
# Dockerfile). With cgo on, Linux builds pull in Wails' gtk4/webkitgtk bindings,
# which fail on CI runners without the GTK dev packages.
CGO_ENABLED=0 go build -tags server -o desktop/bin/hive-desktop-server ./desktop

# Mock-mode servers keep the gate deterministic and offline. Each browser
# project gets its own feed-mode server (chromium 8931, webkit 8934) so
# read/action state cannot leak across concurrently running projects. The
# onboarding mock backend is also a per-process singleton that stays
# authenticated once its fake device flow grants, so each browser project gets
# its own onboarding instance too (chromium 8932, webkit 8933).
#
# Every server gets a per-run HIVE_DATA_DIR and XDG_CONFIG_HOME under this
# script's temp root. The desktop pipeline DB lives under HIVE_DATA_DIR/desktop,
# while the core Hive action runtime uses HIVE_DATA_DIR itself; exporting the
# same isolated root per child process keeps both stores away from developer
# state and prior e2e runs. XDG_CONFIG_HOME keeps onboarding-mode starter flows
# from seeing a developer's existing desktop config.
tmp_base="${TMPDIR:-/tmp}"
tmp_base="${tmp_base%/}"
E2E_DATA_ROOT="$(mktemp -d "${tmp_base}/hive-desktop-e2e.XXXXXX")"

# Playwright can hard-kill its webServer process group, which bypasses EXIT
# traps. macOS has no setsid utility, so use Perl's POSIX binding to detach this
# tiny cleanup watcher; it only removes this run's private temporary root.
perl -MPOSIX=setsid -e '
my ($parent_pid, $data_root) = @ARGV;
setsid();
$SIG{TERM} = "IGNORE";
$SIG{INT} = "IGNORE";
while (kill 0, $parent_pid) {
  sleep 1;
}
system("rm", "-rf", $data_root);
' "$$" "${E2E_DATA_ROOT}" >/dev/null 2>&1 &
cleanup_watcher="$!"
disown "${cleanup_watcher}" 2>/dev/null || true

pids=()
pid_names=()
pid_ports=()
cleanup() {
  trap - EXIT INT TERM
  if [[ ${#pids[@]} -gt 0 ]]; then
    kill "${pids[@]}" 2>/dev/null || true
    wait "${pids[@]}" 2>/dev/null || true
  fi
  rm -rf "${E2E_DATA_ROOT}"
  kill "${cleanup_watcher}" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 143' INT TERM

check_port_free() {
  local port="$1"
  # Refuse a stale mock server rather than accidentally reusing its state.
  if curl -fs -o /dev/null "http://127.0.0.1:${port}/"; then
    echo "error: port ${port} is already in use (stale server from a previous run?)" >&2
    exit 1
  fi
}

start_server() {
  local mode="$1"
  local port="$2"
  local name="$3"
  local data_dir="${E2E_DATA_ROOT}/${name}"
  local config_home="${data_dir}/config"

  mkdir -p "${data_dir}" "${config_home}"
  echo "starting ${mode} mock server ${name} on port ${port}" >&2
  if [[ "${mode}" == "feed" ]]; then
    env \
      HIVE_DATA_DIR="${data_dir}" \
      XDG_CONFIG_HOME="${config_home}" \
      HIVE_DESKTOP_MOCK="${mode}" \
      HIVE_DESKTOP_FLOWS="${FEED_FLOWS_DIR}" \
      HIVE_DESKTOP_ACTIONS="${FEED_ACTIONS_PATH}" \
      WAILS_SERVER_PORT="${port}" \
      desktop/bin/hive-desktop-server &
  elif [[ "${mode}" == "onboarding" ]]; then
    env \
      -u HIVE_DESKTOP_FLOWS \
      -u HIVE_DESKTOP_ACTIONS \
      HIVE_DATA_DIR="${data_dir}" \
      XDG_CONFIG_HOME="${config_home}" \
      HIVE_DESKTOP_MOCK="${mode}" \
      WAILS_SERVER_PORT="${port}" \
      desktop/bin/hive-desktop-server &
  else
    echo "error: unknown mock server mode ${mode}" >&2
    exit 1
  fi

  pids+=("$!")
  pid_names+=("${name}")
  pid_ports+=("${port}")
}

wait_ready() {
  local pid="$1"
  local name="$2"
  local port="$3"

  for _ in $(seq 1 50); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      echo "error: ${name} server on port ${port} exited before becoming ready (bind failure?)" >&2
      exit 1
    fi
    if curl -fs -o /dev/null "http://127.0.0.1:${port}/"; then
      return 0
    fi
    sleep 0.2
  done

  echo "error: ${name} server on port ${port} did not become ready" >&2
  exit 1
}

for port in 8931 8932 8933 8934; do
  check_port_free "${port}"
done

# Start every non-sentinel mock server first. Playwright waits only on the
# chromium feed URL (8931), so that server is started last after the webkit feed
# and both onboarding servers are already ready.
start_server onboarding 8932 onboarding-chromium
start_server onboarding 8933 onboarding-webkit
start_server feed 8934 feed-webkit

for i in "${!pids[@]}"; do
  wait_ready "${pids[$i]}" "${pid_names[$i]}" "${pid_ports[$i]}"
done

start_server feed 8931 feed-chromium
last_index=$((${#pids[@]} - 1))
wait_ready "${pids[$last_index]}" "${pid_names[$last_index]}" "${pid_ports[$last_index]}"

# Keep this process alive for Playwright, and reap children plus temporary data
# on normal teardown.
while true; do
  running=$(jobs -pr | wc -l | tr -d ' ')
  if [[ "${running}" -lt "${#pids[@]}" ]]; then
    echo "error: a desktop mock server exited unexpectedly" >&2
    exit 1
  fi
  sleep 1
done
