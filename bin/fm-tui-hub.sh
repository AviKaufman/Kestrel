#!/usr/bin/env bash
# Resolve and send to the existing Firstmate primary supervisor endpoint.
# Usage:
#   fm-tui-hub.sh resolve
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

# shellcheck source=bin/fm-supervisor-target-lib.sh
. "$SCRIPT_DIR/fm-supervisor-target-lib.sh"
# shellcheck source=bin/fm-backend.sh
. "$SCRIPT_DIR/fm-backend.sh"

resolve_hub() {
  local backend target
  if ! target=$(discover_supervisor_target); then
    echo "error: Firstmate hub target is unavailable; set FM_SUPERVISOR_TARGET explicitly" >&2
    return 1
  fi
  if ! backend=$(discover_supervisor_backend); then
    echo "error: Firstmate hub backend is unavailable; set FM_SUPERVISOR_BACKEND explicitly" >&2
    return 1
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
    echo "usage: fm-tui-hub.sh resolve | send <backend> <target> <message>" >&2
    exit 2
    ;;
esac
