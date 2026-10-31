#!/usr/bin/env bash
set -euo pipefail

# Refuse to start outside run-docker.sh, so a host `npx playwright test` cannot
# fall back to local servers.
if [[ ! "${HIVE_DESKTOP_E2E_HARNESS:-}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo "error: desktop e2e must run through mise run desktop:e2e (Docker harness marker missing)" >&2
  exit 1
fi

cd "$(dirname "$0")/../../../.."
# run-docker.sh prebuilds the server, so the webServer timeout covers startup only.
if [[ ! -x cmd/desktop/bin/hive-desktop-server ]]; then
  echo "error: cmd/desktop/bin/hive-desktop-server missing; run-docker.sh must prebuild it (cmd/desktop/e2e/scripts/run-docker.sh)" >&2
  exit 1
fi

# Each server gets a private copy of testdata/, so editor writes and watcher
# reloads never touch checked-in files or another server.
tmp_base="${TMPDIR:-/tmp}"
tmp_base="${tmp_base%/}"
E2E_DATA_ROOT="$(mktemp -d "${tmp_base}/hive-desktop-e2e.XXXXXX")"
# Action ids are slug-limited to 64 bytes. Derive a short, deterministic id
# from this unique temp root rather than embedding its long basename.
RUN_ID="$(printf '%s' "${E2E_DATA_ROOT}" | cksum | awk '{print "r" $1}')"
FIXTURES="$(pwd)/cmd/desktop/e2e/testdata"

pids=()
pid_names=()
pid_ports=()
cleanup() {
  trap - EXIT INT TERM
  if ((${#pids[@]})); then
    kill "${pids[@]}" 2>/dev/null || true
    wait "${pids[@]}" 2>/dev/null || true
  fi
  rm -rf "${E2E_DATA_ROOT}"
}
trap cleanup EXIT
trap 'exit 143' INT TERM

check_port_free() {
  local port="$1"
  if curl -fs -o /dev/null "http://127.0.0.1:${port}/"; then
    echo "error: port ${port} is already in use" >&2
    exit 1
  fi
}

prepare_action_runtime() {
  local data_dir="$1"
  local source="${data_dir}/workspace/fixture"
  local remote="${data_dir}/remote.git"
  mkdir -p "${data_dir}/workspace"
  git init "${source}" >/dev/null
  git -C "${source}" config user.email e2e@hive.invalid
  git -C "${source}" config user.name 'Hive E2E'
  printf 'hive desktop e2e fixture\n' >"${source}/README.md"
  git -C "${source}" add README.md
  git -C "${source}" commit -m fixture >/dev/null
  git -C "${source}" branch -M main
  git init --bare "${remote}" >/dev/null
  git -C "${source}" remote add origin "${remote}"
  git -C "${source}" push -u origin main >/dev/null
  git --git-dir="${remote}" symbolic-ref HEAD refs/heads/main

  # The window command is deliberately harmless. Session creation still runs
  # the real clone/save path, but no agent process starts.
  cat >"${data_dir}/hive-e2e.yaml" <<EOF
version: "0.2.7"
git_path: git
workspaces:
  - "${data_dir}/workspace"
agents:
  default: smoke
  smoke:
    command: /usr/bin/true
rules:
  - pattern: ".*"
    windows:
      - name: smoke
        command: /usr/bin/true
EOF
}

# The remote lane's server lists two servers: server-a from the ssh fixture,
# reached by name on the fixture's Docker network, and one that refuses every
# connection. run-docker.sh mounts the fixture's keys at /remote.
prepare_remote_runtime() {
  local data_dir="$1"
  local ssh_dir="${data_dir}/ssh"
  mkdir -p "${ssh_dir}"
  chmod 700 "${ssh_dir}"
  # ssh refuses a key that others can read, which a read-only mount can be.
  install -m 600 /remote/id_ed25519 "${ssh_dir}/id_ed25519"
  awk '{print "server-a", $1, $2}' /remote/server-a_host_ed25519_key.pub >"${ssh_dir}/known_hosts"
  cat >"${ssh_dir}/config" <<EOF
Host server-a
  HostName server-a
  User hive
  IdentityFile ${ssh_dir}/id_ed25519
  UserKnownHostsFile ${ssh_dir}/known_hosts
  IdentitiesOnly yes

Host gone
  HostName 127.0.0.1
  Port 9
  User hive
  IdentityFile ${ssh_dir}/id_ed25519
  UserKnownHostsFile ${ssh_dir}/known_hosts
EOF
  cat >"${data_dir}/hive-e2e.yaml" <<EOF
version: "0.2.7"
git_path: git
servers:
  - name: server-a
    ssh: server-a
    ssh_config: ${ssh_dir}/config
  - name: gone
    ssh: gone
    ssh_config: ${ssh_dir}/config
EOF
}

prepare_server_files() {
  local data_dir="$1" mode="$2" name="$3"
  local fixture_dir="${data_dir}/fixtures"
  mkdir -p "${fixture_dir}"
  cp -R "${FIXTURES}/flows" "${fixture_dir}/flows"
  case "${mode}" in
    pipeline) rm -rf "${fixture_dir}/flows"; mkdir -p "${fixture_dir}/flows"; cp -R "${FIXTURES}/flows/source-to-commit/." "${fixture_dir}/flows/" ;;
  esac
  if [[ "${mode}" == "action-smoke" || "${mode}" == "action-seed" ]]; then
    prepare_action_runtime "${data_dir}"
  fi
  if [[ "${name}" == "remote" ]]; then
    prepare_remote_runtime "${data_dir}"
  fi
  if [[ "${mode}" == "action-smoke" ]]; then
    # Run-scoped IDs, session names, and topic keep foreign rows out of the
    # smoke reader.
    sed -e "s|__RUN_ID__|${RUN_ID}-${mode}|g" \
        -e "s|__REMOTE__|${data_dir}/remote.git|g" \
        -e "s|__MISSING_REMOTE__|${data_dir}/missing.git|g" \
        "${FIXTURES}/actions-smoke.yml" >"${fixture_dir}/actions.yml"
  elif [[ "${mode}" != "action-seed" ]]; then
    cp "${FIXTURES}/actions.yml" "${fixture_dir}/actions.yml"
  fi
  if [[ "${mode}" != "onboarding" ]]; then
    # hive-e2e.yaml sets no context.base_dir, so the drive root is
    # ${data_dir}/context and this is the e2e/repo space.
    local space_dir="${data_dir}/context/e2e/repo"
    mkdir -p "${space_dir}/plans" "${space_dir}/canvases" "${space_dir}/pages"
    cp "${FIXTURES}/drive/plans/2026-10-11-plan.md" "${space_dir}/plans/"
    cp "${FIXTURES}/drive/canvases/report.canvas.json" "${space_dir}/canvases/"
    cp "${FIXTURES}/drive/pages/links.html" "${space_dir}/pages/"
  fi
}

start_server() {
  local mode="$1" port="$2" name="$3"
  local data_dir="${E2E_DATA_ROOT}/${name}"
  local config_home="${data_dir}/config"
  mkdir -p "${data_dir}" "${config_home}"
  prepare_server_files "${data_dir}" "${mode}" "${name}"
  if [[ ! -f "${data_dir}/hive-e2e.yaml" ]]; then
    cat >"${data_dir}/hive-e2e.yaml" <<EOF
version: "0.2.7"
git_path: git
EOF
  fi
  echo "starting ${mode} mock server ${name} on port ${port}" >&2
  if [[ "${mode}" == "onboarding" ]]; then
    env -u HIVE_DESKTOP_FLOWS_DIR -u HIVE_DESKTOP_ACTIONS_PATH \
      HIVE_DESKTOP_DATA_DIR="${data_dir}" HIVE_DESKTOP_HIVE_DATA_DIR="${data_dir}" \
      XDG_CONFIG_HOME="${config_home}" \
      HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE="${mode}" WAILS_SERVER_PORT="${port}" \
      cmd/desktop/bin/hive-desktop-server &
  else
    mkdir -p "${config_home}/hive/desktop"
    cat >"${config_home}/hive/desktop/settings.yaml" <<EOF
onboarding:
  completed: true
EOF
    local action_path="${data_dir}/fixtures/actions.yml"
    mkdir -p "${config_home}/hive/desktop"
    cat >"${config_home}/hive/desktop/settings.yaml" <<EOF
onboarding:
  completed: true
EOF
    env \
      HIVE_DESKTOP_DATA_DIR="${data_dir}" \
      HIVE_DESKTOP_HIVE_DATA_DIR="${data_dir}" \
      HIVE_DESKTOP_HTTP_PORT=0 \
      HIVE_CONFIG="${data_dir}/hive-e2e.yaml" \
      XDG_CONFIG_HOME="${config_home}" \
      HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE="$([[ "${mode}" == "action-seed" ]] && echo action-smoke || echo "${mode}")" \
      HIVE_DESKTOP_FLOWS_DIR="${data_dir}/fixtures/flows" \
      HIVE_DESKTOP_ACTIONS_PATH="${action_path}" \
      HIVE_DESKTOP_SMOKE_RUN_ID="${RUN_ID}-${mode}" \
      HIVE_DESKTOP_SMOKE_REMOTE="${data_dir}/remote.git" \
      WAILS_SERVER_PORT="${port}" \
      cmd/desktop/bin/hive-desktop-server &
  fi
  pids+=("$!")
  pid_names+=("${name}")
  pid_ports+=("${port}")
}

wait_ready() {
  local pid="$1" name="$2" port="$3"
  for _ in $(seq 1 100); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      echo "error: ${name} server on port ${port} exited before becoming ready" >&2
      exit 1
    fi
    if curl -fs -o /dev/null "http://127.0.0.1:${port}/"; then return 0; fi
    sleep 0.2
  done
  echo "error: ${name} server on port ${port} did not become ready" >&2
  exit 1
}

# Ports must match fixtures/servers.ts. The chromium feed server starts last
# because playwright.config.ts polls it as the readiness sentinel.
for port in 8931 8932 8933 8934 8935 8936 8937 8938; do check_port_free "${port}"; done
start_server onboarding 8932 onboarding-chromium
start_server onboarding 8933 onboarding-webkit
start_server feed 8934 feed-webkit
start_server pipeline 8935 pipeline-smoke
start_server action-smoke 8936 action-smoke
# This server starts with the private actions.yml absent. It proves exact
# first-run seeding without ever copying an action fixture.
start_server action-seed 8937 action-seed
# Only the remote lane can reach the ssh fixture.
if [[ -n "${HIVE_DESKTOP_E2E_REMOTE:-}" ]]; then
  start_server feed 8938 remote
fi
for i in "${!pids[@]}"; do wait_ready "${pids[$i]}" "${pid_names[$i]}" "${pid_ports[$i]}"; done
start_server feed 8931 feed-chromium
last_index=$((${#pids[@]} - 1))
wait_ready "${pids[$last_index]}" "${pid_names[$last_index]}" "${pid_ports[$last_index]}"

# Do not use jobs/disown/Perl: those assumptions differ between macOS and the
# Linux image. A direct liveness loop is portable and keeps cleanup ownership.
while true; do
  for i in "${!pids[@]}"; do
    if ! kill -0 "${pids[$i]}" 2>/dev/null; then
      echo "error: ${pid_names[$i]} server exited unexpectedly" >&2
      exit 1
    fi
  done
  sleep 1
done
