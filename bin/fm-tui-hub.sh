#!/usr/bin/env bash
# Resolve and send to the existing Firstmate primary supervisor endpoint.
# Usage:
#   fm-tui-hub.sh resolve
#   fm-tui-hub.sh history <backend> <target> <lines>
#   fm-tui-hub.sh send <backend> <target> <message>
#
# bin/fm-supervisor-target-lib.sh owns supervisor discovery.
# bin/fm-backend.sh owns target existence and verified submission.
set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -z "${FM_HOME:-}" ]; then
  echo "error: FM_HOME is required" >&2
  exit 1
fi
STATE="${FM_STATE_OVERRIDE:-$FM_HOME/state}"
[ -d "$STATE" ] || { echo "error: state dir '$STATE' is missing" >&2; exit 1; }

# shellcheck source=bin/fm-supervisor-target-lib.sh
. "$SCRIPT_DIR/fm-supervisor-target-lib.sh"
# shellcheck source=bin/fm-backend.sh
. "$SCRIPT_DIR/fm-backend.sh"

resolve_hub() {
  local backend target locked
  if [ -z "${FM_SUPERVISOR_TARGET:-}" ] && [ -z "${FM_SUPERVISOR_BACKEND:-}" ] \
    && locked=$(discover_locked_supervisor_context "$STATE"); then
    IFS=$'\t' read -r backend target <<EOF
$locked
EOF
  else
    if ! target=$(discover_supervisor_target); then
      echo "error: Firstmate hub target is unavailable; no valid active primary session was found" >&2
      return 1
    fi
    if ! backend=$(discover_supervisor_backend); then
      echo "error: Firstmate hub backend is unavailable; no valid active primary session was found" >&2
      return 1
    fi
  fi
  case "$backend" in
    tmux|herdr) ;;
    *)
      echo "error: Firstmate hub backend '$backend' is unsupported (expected tmux or herdr)" >&2
      return 1
      ;;
  esac
  if ! fm_backend_target_exists "$backend" "$target"; then
    echo "error: Firstmate hub target '$target' does not resolve to a live $backend pane" >&2
    return 1
  fi
  printf '%s\t%s\n' "$backend" "$target"
}

case "${1:-}" in
  resolve)
    [ "$#" -eq 1 ] || { echo "error: resolve takes no arguments" >&2; exit 2; }
    resolve_hub
    ;;
  history)
    [ "$#" -eq 4 ] || { echo "error: history requires backend, target, and line bound" >&2; exit 2; }
    requested_backend=$2
    requested_target=$3
    lines=$4
    case "$lines" in
      ''|*[!0-9]*|0) echo "error: history lines must be positive" >&2; exit 1 ;;
    esac
    [ "$lines" -le 200 ] || { echo "error: history lines exceeds 200" >&2; exit 1; }
    current=$(resolve_hub) || exit 1
    [ "$current" = "$requested_backend	$requested_target" ] || {
      echo "error: Firstmate hub target changed before history capture; refresh and retry" >&2
      exit 1
    }
    fm_backend_capture "$requested_backend" "$requested_target" "$lines"
    ;;
  send)
    [ "$#" -eq 4 ] || { echo "error: send requires backend, target, and one message argument" >&2; exit 2; }
    requested_backend=$2
    requested_target=$3
    message=$4
    [ -n "${message//[[:space:]]/}" ] || { echo "error: message must not be empty" >&2; exit 1; }
    message_bytes=$(LC_ALL=C; printf '%s' "$message" | wc -c | tr -d '[:space:]')
    [ "$message_bytes" -le 4096 ] || { echo "error: message exceeds 4096-byte limit" >&2; exit 1; }
    current=$(resolve_hub) || exit 1
    [ "$current" = "$requested_backend	$requested_target" ] || {
      echo "error: Firstmate hub target changed before send; refresh and retry" >&2
      exit 1
    }
    case "$message" in
      /*) settle=1.2 ;;
      \$*)
        if [ "$("$SCRIPT_DIR/fm-harness.sh")" = codex ]; then settle=1.2; else settle=0.3; fi
        ;;
      *) settle=0.3 ;;
    esac
    verdict=$(fm_backend_send_text_submit "$requested_backend" "$requested_target" "$message" \
      "${FM_SEND_RETRIES:-3}" "${FM_SEND_SLEEP:-0.4}" "$settle")
    case "$verdict" in
      pending)
        echo "error: text not submitted to Firstmate hub (Enter swallowed; text left in composer)" >&2
        exit 1
        ;;
      send-failed)
        echo "error: text not sent to Firstmate hub" >&2
        exit 1
        ;;
    esac
    printf 'sent\n'
    ;;
  *)
    echo "usage: fm-tui-hub.sh resolve | history <backend> <target> <lines> | send <backend> <target> <message>" >&2
    exit 2
    ;;
esac
