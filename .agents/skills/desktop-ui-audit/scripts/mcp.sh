#!/usr/bin/env bash
# Drive the native dev app through its Wails MCP server. See ../SKILL.md.
#
#   mcp.sh status                      is it up, which route, how many test ids
#   mcp.sh tools                       the tool table
#   mcp.sh call <tool> [args-json]     any tool, result unwrapped
#   mcp.sh js <code>                   async function body; `return` a JSON value
#   mcp.sh testids [substring]         data-testids in the DOM, with visibility and text
#   mcp.sh route                       current hash route and title
#   mcp.sh text [selector]             visible text of an element (default: body)
#   mcp.sh html <selector>             outerHTML of the first match
#   mcp.sh query <selector> [limit]    elements: tag, text, value, bounds, visible
#   mcp.sh snapshot [depth]            structural outline of the viewport
#   mcp.sh click <selector>            animated click (left button, single)
#   mcp.sh type <selector> <text>      click, then type with per-key events ("-": the focused element)
#   mcp.sh press <key> [mod,mod]       e.g. Enter, Escape, k meta
#   mcp.sh scroll <selector> [deltaY]  wheel event at the element (default 240)
#   mcp.sh wait <selector> [seconds]   poll until a visible match exists (default 10s)
#
# The endpoint comes from launch.env (WAILS_MCP_HOST/PORT); override with
# WAILS_MCP_URL or point HIVE_LAUNCH_ENV at launch.onboarding.env.
set -euo pipefail

usage() { sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; }

read_key() { sed -nE "s/^$2=\"?([^\"]*)\"?\$/\1/p" "$1" | head -1; }

root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
env_file=${HIVE_LAUNCH_ENV:-$root/launch.env}
mcp_url=${WAILS_MCP_URL:-}

ensure_url() {
  [[ -n $mcp_url ]] && return
  [[ -f $env_file ]] || { echo "no $env_file; run 'mise run desktop:dev:prepare'" >&2; exit 2; }
  local host port
  port=$(read_key "$env_file" WAILS_MCP_PORT)
  [[ -n $port ]] || { echo "$env_file has no WAILS_MCP_PORT; run 'mise run desktop:dev:prepare' to regenerate it" >&2; exit 2; }
  host=$(read_key "$env_file" WAILS_MCP_HOST)
  mcp_url="http://${host:-127.0.0.1}:$port/mcp"
}

rpc() { # method [params-json]
  ensure_url
  curl -sS --max-time "${MCP_TIMEOUT:-40}" "$mcp_url" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":${2:-"{}"}}"
}

# call <tool> [args-json]: prints the tool's value, pretty-printed when it is
# JSON. A transport or tool error goes to stderr with exit 1.
call() {
  local response text
  response=$(rpc tools/call "{\"name\":\"$1\",\"arguments\":${2:-"{}"}}") || {
    echo "no answer from $mcp_url; is 'mise run desktop:dev' running?" >&2; return 1; }
  if [[ -n $(jq -r '.error.message // empty' <<<"$response") ]]; then
    jq -r '.error.message' <<<"$response" >&2; return 1
  fi
  text=$(jq -r '.result.content[0].text // empty' <<<"$response")
  if [[ $(jq -r '.result.isError // false' <<<"$response") == true ]]; then
    echo "$text" >&2; return 1
  fi
  if jq -e . >/dev/null 2>&1 <<<"$text"; then jq . <<<"$text"; else printf '%s\n' "$text"; fi
}

js() { call js_eval "$(jq -n --arg js "$1" '{js: $js}')"; }

sel_args() { jq -n --arg s "$1" --argjson extra "${2:-{\}}" '{selector: $s} + $extra'; }

cmd=${1:-}; shift || true
case $cmd in
  status)
    ensure_url
    curl -sf --max-time 3 "${mcp_url%/mcp}/" >/dev/null || {
      echo "MCP server not answering at $mcp_url; is 'mise run desktop:dev' running?" >&2; exit 1; }
    page=$(js 'return {route: location.hash, title: document.title, testids: document.querySelectorAll("[data-testid]").length, viewport: {width: innerWidth, height: innerHeight}, focused: document.activeElement && document.activeElement.dataset.testid || null}')
    api=null
    if [[ -f $env_file ]]; then
      http_port=$(read_key "$env_file" HIVE_DESKTOP_HTTP_PORT)
      [[ -n $http_port ]] && api=$(curl -sf --max-time 3 "http://127.0.0.1:$http_port/api/status" || echo null)
    fi
    jq -n --arg mcp "$mcp_url" --argjson page "$page" --argjson api "$api" '{mcp: $mcp, page: $page, hive_api: $api}'
    ;;
  tools)   rpc tools/list | jq -r '.result.tools[] | "\(.name)\t\(.description | split(". ")[0])"' ;;
  call)    [[ $# -ge 1 ]] || { usage; exit 2; }; call "$1" "${2:-{\}}" ;;
  js)      [[ $# -eq 1 ]] || { usage; exit 2; }; js "$1" ;;
  testids)
    js "const f = $(jq -n --arg f "${1:-}" '$f');
        return [...document.querySelectorAll('[data-testid]')].map(e => {
          const r = e.getBoundingClientRect();
          return {id: e.dataset.testid, tag: e.tagName.toLowerCase(), visible: r.width > 0 && r.height > 0,
                  text: (e.innerText || e.getAttribute('aria-label') || e.value || '').trim().replace(/\s+/g, ' ').slice(0, 60)};
        }).filter(x => !f || x.id.includes(f));"
    ;;
  route)   js 'return {route: location.hash, title: document.title}' ;;
  text)
    js "const el = document.querySelector($(jq -n --arg s "${1:-body}" '$s'));
        if (!el) throw new Error('no element matches');
        return el.innerText.replace(/\s+/g, ' ').trim().slice(0, 4000);"
    ;;
  html)    [[ $# -ge 1 ]] || { usage; exit 2; }; call dom_html "$(sel_args "$1")" ;;
  query)   [[ $# -ge 1 ]] || { usage; exit 2; }; call dom_query "$(sel_args "$1" "{\"limit\": ${2:-25}}")" ;;
  snapshot) call screenshot_dom "{\"max_depth\": ${1:-12}}" ;;
  click)   [[ $# -ge 1 ]] || { usage; exit 2; }; call mouse_click "$(sel_args "$1")" ;;
  type)    [[ $# -eq 2 ]] || { usage; exit 2; }
           call keyboard_type "$(jq -n --arg s "$1" --arg t "$2" 'if $s == "-" then {text: $t} else {selector: $s, text: $t} end')" ;;
  press)   [[ $# -ge 1 ]] || { usage; exit 2; }; call keyboard_press "$(jq -n --arg k "$1" --arg m "${2:-}" '{key: $k, modifiers: ($m | split(",") | map(select(. != "")))}')" ;;
  scroll)  [[ $# -ge 1 ]] || { usage; exit 2; }; call mouse_scroll "$(sel_args "$1" "{\"delta_y\": ${2:-240}}")" ;;
  wait)
    [[ $# -ge 1 ]] || { usage; exit 2; }
    deadline=$(( $(date +%s) + ${2:-10} ))
    while :; do
      n=$(call dom_query "$(sel_args "$1" '{"limit": 1}')" | jq '[.elements[] | select(.visible)] | length')
      [[ $n -gt 0 ]] && { echo "visible: $1"; exit 0; }
      [[ $(date +%s) -ge $deadline ]] && { echo "timed out after ${2:-10}s waiting for $1" >&2; exit 1; }
      sleep 0.5
    done
    ;;
  ""|-h|--help|help) usage ;;
  *) echo "unknown command: $cmd" >&2; usage; exit 2 ;;
esac
