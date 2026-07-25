#!/usr/bin/env bash
# Discover, capture, and send to live tmux Codex panes not owned by Firstmate.
# Usage:
#   fm-tui-direct.sh list
#   fm-tui-direct.sh peek <session:window.pane> <lines>
#   fm-tui-direct.sh send <session:window.pane> <message>
#
# Direct sessions are intentionally tmux-only in this slice.
# Firstmate-managed windows are excluded using state/*.meta before any output,
# capture, or send.
set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -z "${FM_HOME:-}" ]; then
  echo "error: FM_HOME is required" >&2
  exit 1
fi
STATE="${FM_STATE_OVERRIDE:-$FM_HOME/state}"
[ -d "$STATE" ] || { echo "error: state dir '$STATE' is missing" >&2; exit 1; }

# shellcheck source=bin/fm-backend.sh
. "$SCRIPT_DIR/fm-backend.sh"

valid_target() {
  [[ $1 =~ ^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+\.[0-9]+$ ]]
}

managed_window() {
  local candidate=$1 meta backend window
  for meta in "$STATE"/*.meta; do
    [ -e "$meta" ] || continue
    backend=$(fm_backend_of_meta "$meta")
    [ "$backend" = tmux ] || continue
    window=$(fm_meta_get "$meta" window)
    [ "$window" = "$candidate" ] && return 0
  done
  return 1
}

codex_command() {
  local command=${1#-}
  case "$command" in
    *codex*) return 0 ;;
    *) return 1 ;;
  esac
}

direct_record() {
  local target=$1 record session window pane command project
  valid_target "$target" || return 1
  record=$(tmux display-message -p -t "$target" \
    '#{session_name}	#{window_name}	#{pane_index}	#{pane_current_command}	#{pane_current_path}' 2>/dev/null) \
    || return 1
  IFS=$'\t' read -r session window pane command project <<EOF
$record
EOF
  [ "$target" = "$session:$window.$pane" ] || return 1
  codex_command "$command" || return 1
  managed_window "$session:$window" && return 1
  printf '%s\t%s\n' "$target" "$project"
}

case "${1:-}" in
  list)
    tmux list-panes -a -F \
      '#{session_name}	#{window_name}	#{pane_index}	#{pane_current_command}	#{pane_current_path}' 2>/dev/null \
      | LC_ALL=C sort \
      | while IFS=$'\t' read -r session window pane command project; do
          target="$session:$window.$pane"
          valid_target "$target" || continue
          codex_command "$command" || continue
          managed_window "$session:$window" && continue
          printf '%s\t%s\n' "$target" "$project"
        done
    ;;
  peek)
    target=${2:-}
    lines=${3:-}
    valid_target "$target" || { echo "error: invalid direct target '$target'" >&2; exit 1; }
    case "$lines" in
      ''|*[!0-9]*|0) echo "error: lines must be positive" >&2; exit 1 ;;
    esac
    [ "$lines" -le 200 ] || { echo "error: lines exceeds 200" >&2; exit 1; }
    direct_record "$target" >/dev/null \
      || { echo "error: target '$target' is not a live private Codex pane" >&2; exit 1; }
    fm_backend_capture tmux "$target" "$lines"
    ;;
  send)
    target=${2:-}
    message=${3:-}
    [ "$#" -eq 3 ] || { echo "error: send requires one target and one message argument" >&2; exit 1; }
    valid_target "$target" || { echo "error: invalid direct target '$target'" >&2; exit 1; }
    [ -n "$message" ] || { echo "error: message must not be empty" >&2; exit 1; }
    message_bytes=$(LC_ALL=C; printf '%s' "$message" | wc -c | tr -d '[:space:]')
    [ "$message_bytes" -le 4096 ] || { echo "error: message exceeds 4096-byte limit" >&2; exit 1; }
    direct_record "$target" >/dev/null \
      || { echo "error: target '$target' is not a live private Codex pane" >&2; exit 1; }
    case "$message" in
      /*|\$*) settle=1.2 ;;
      *) settle=0.3 ;;
    esac
    verdict=$(fm_backend_send_text_submit tmux "$target" "$message" \
      "${FM_SEND_RETRIES:-3}" "${FM_SEND_SLEEP:-0.4}" "$settle")
    case "$verdict" in
      pending)
        echo "error: text not submitted to $target (Enter swallowed; text left in composer)" >&2
        exit 1
        ;;
      send-failed)
        echo "error: text not sent to $target" >&2
        exit 1
        ;;
    esac
    printf 'sent\n'
    ;;
  *)
    echo "usage: fm-tui-direct.sh list | peek <target> <lines> | send <target> <message>" >&2
    exit 2
    ;;
esac
