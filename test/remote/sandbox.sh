#!/usr/bin/env bash
# A sandbox of hive servers on this machine, for trying remote sessions by
# hand: N containers from the test/remote image, each running hive behind sshd
# on 127.0.0.1. Hive Desktop and the CLI reach them as servers under servers:.
#
#   up [--claude] [N]
#            start N servers (default 3), replacing the ones that run;
#            --claude runs real Claude Code with the token in
#            $state/claude-token (from `claude setup-token`)
#   status   list the servers and check each one over ssh
#   down     remove the servers
#
# The state (keys, ssh_config) lives outside the checkout, so the servers:
# block that up prints keeps working from any worktree. Set
# HIVE_SANDBOX_STATE to move it and HIVE_SANDBOX_PORT for the first port.
set -euo pipefail
cd "$(dirname "$0")/../.."

state="${HIVE_SANDBOX_STATE:-${XDG_DATA_HOME:-$HOME/.local/share}/hive-sandbox}"
base_port="${HIVE_SANDBOX_PORT:-2221}"
image="hive-sandbox:local"
label="dev.hive.sandbox=1"

containers() {
  docker ps -a --filter "label=${label}" --format '{{.Names}}' | sort -V
}

down() {
  local names
  names="$(containers)"
  if [[ -n "${names}" ]]; then
    # shellcheck disable=SC2086 # one name per word
    docker rm -f ${names} >/dev/null
  fi
  rm -f "${state}/ssh_config" "${state}/known_hosts"
}

wait_for_sshd() {
  local name="$1"
  for _ in $(seq 1 60); do
    if docker exec "${name}" bash -c 'exec 3<>/dev/tcp/127.0.0.1/22' 2>/dev/null; then return 0; fi
    sleep 0.5
  done
  echo "error: sshd in ${name} did not start" >&2
  docker logs "${name}" >&2 || true
  exit 1
}

up() {
  local claude=""
  if [[ ${1:-} == --claude ]]; then
    claude=1
    shift
  fi
  local count="${1:-3}"
  if ! [[ "${count}" =~ ^[1-9][0-9]?$ ]]; then
    echo "error: the server count must be 1 to 99, not ${count}" >&2
    exit 2
  fi
  down
  mkdir -p "${state}"
  chmod 700 "${state}"
  [[ -f "${state}/id_ed25519" ]] || ssh-keygen -q -t ed25519 -N '' -C hive-sandbox -f "${state}/id_ed25519"
  docker build --quiet --file test/remote/Dockerfile --tag "${image}" . >/dev/null
  local run_image="${image}" token_mount=()
  if [[ -n ${claude} ]]; then
    if [[ ! -s "${state}/claude-token" ]]; then
      echo "error: --claude needs a token in ${state}/claude-token; run claude setup-token and save it there" >&2
      exit 1
    fi
    run_image="hive-sandbox-claude:local"
    docker build --quiet --file test/remote/sandbox-claude/Dockerfile --tag "${run_image}" . >/dev/null
    token_mount=(--volume "${state}/claude-token:/run/claude-token:ro")
  fi

  : >"${state}/known_hosts"
  : >"${state}/ssh_config"
  for i in $(seq 1 "${count}"); do
    local name="sandbox-${i}" port=$((base_port + i - 1))
    local host_key="${state}/${name}_host_ed25519_key"
    [[ -f "${host_key}" ]] || ssh-keygen -q -t ed25519 -N '' -f "${host_key}"
    docker run --detach --name "hive-${name}" --hostname "${name}" --label "${label}" \
      --publish "127.0.0.1:${port}:22" \
      --volume "${state}/id_ed25519.pub:/run/client.pub:ro" \
      --volume "${host_key}:/run/host_key:ro" \
      ${token_mount[@]+"${token_mount[@]}"} \
      "${run_image}" >/dev/null
    awk -v host="[127.0.0.1]:${port}" '{print host, $1, $2}' "${host_key}.pub" >>"${state}/known_hosts"
    cat >>"${state}/ssh_config" <<CONFIG
Host ${name}
  HostName 127.0.0.1
  Port ${port}
  User hive
  IdentityFile "${state}/id_ed25519"
  UserKnownHostsFile "${state}/known_hosts"
  IdentitiesOnly yes

CONFIG
  done
  for i in $(seq 1 "${count}"); do wait_for_sshd "hive-sandbox-${i}"; done

  cat <<EOF
${count} hive servers are running. Add them to the config.yaml that Hive Desktop
and the hive CLI read (~/.config/hive/config.yaml, or the file HIVE_CONFIG names):

servers:
EOF
  for i in $(seq 1 "${count}"); do
    cat <<EOF
  - name: sandbox-${i}
    ssh: sandbox-${i}
    ssh_config: ${state}/ssh_config
EOF
  done
  if [[ -n ${claude} ]]; then
    cat <<EOF

Each server runs Claude Code with --dangerously-skip-permissions and lists two
repositories: hay-kot/appkit and hay-kot/criterio.

Try one:  ssh -F ${state}/ssh_config sandbox-1
Remove them with: mise run remote:down
EOF
    return
  fi
  cat <<EOF

Each server has two repositories to create sessions from:
  /home/hive/repo.git     the spawn rule runs claude in a tmux session
  /home/hive/windows.git  the windows rule opens an "agent" window
claude on the servers is a stand-in that draws a fixed screen.

Try one:  ssh -F ${state}/ssh_config sandbox-1
Or add "Include ${state}/ssh_config" to ~/.ssh/config and drop ssh_config: above.
Remove them with: mise run remote:down
EOF
}

status() {
  local names
  names="$(containers)"
  if [[ -z "${names}" ]]; then
    echo "no sandbox servers are running"
    return
  fi
  for container in ${names}; do
    local name="${container#hive-}"
    if ssh -F "${state}/ssh_config" -o BatchMode=yes -o ConnectTimeout=5 "${name}" -- hive --version >/dev/null 2>&1; then
      echo "${name}: up"
    else
      echo "${name}: not answering"
    fi
  done
}

cmd="${1:-up}"
shift || true
case "${cmd}" in
  up) up "$@" ;;
  status) status ;;
  down) down ;;
  *)
    echo "usage: sandbox.sh up [N]|status|down" >&2
    exit 2
    ;;
esac
