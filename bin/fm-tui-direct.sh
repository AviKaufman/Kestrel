#!/usr/bin/env bash
# Discover, capture, and send to live tmux Codex panes not owned by Firstmate.
# Usage:
#   fm-tui-direct.sh list
#   fm-tui-direct.sh peek <session:window.pane> <lines>
#   fm-tui-direct.sh send <session:window.pane> <message>
#   fm-tui-direct.sh create <label> <absolute-directory>
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

valid_label() {
  [[ $1 =~ ^[a-z0-9][a-z0-9-]{0,31}$ ]]
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
  create)
    label=${2:-}
    workdir=${3:-}
    [ "$#" -eq 3 ] || { echo "error: create requires one label and one directory argument" >&2; exit 1; }
    valid_label "$label" || { echo "error: invalid private label '$label'" >&2; exit 1; }
    case "$workdir" in
      /*) ;;
      *) echo "error: private directory must be absolute" >&2; exit 1 ;;
    esac
    case "$workdir" in
      *$'\t'*|*$'\r'*|*$'\n'*) echo "error: private directory contains control characters" >&2; exit 1 ;;
    esac
    [ -d "$workdir" ] || { echo "error: private directory '$workdir' is unavailable" >&2; exit 1; }
    codex_bin=$(command -v codex 2>/dev/null) \
      || { echo "error: codex command is unavailable" >&2; exit 1; }
    [ -x "$codex_bin" ] || { echo "error: codex command '$codex_bin' is not executable" >&2; exit 1; }
    [[ $codex_bin =~ ^/[A-Za-z0-9_./+-]+$ ]] \
      || { echo "error: codex command path is not safe for tmux launch" >&2; exit 1; }

    if [ -n "${TMUX:-}" ]; then
      session=$(tmux display-message -p '#S') \
        || { echo "error: cannot resolve current tmux session" >&2; exit 1; }
    else
      session=firstmate-private
    fi
    [[ $session =~ ^[A-Za-z0-9_.-]+$ ]] \
      || { echo "error: invalid tmux session '$session'" >&2; exit 1; }
    window="codex-$label"
    if tmux has-session -t "$session" 2>/dev/null; then
      if tmux list-windows -t "$session" -F '#{window_name}' | grep -qx "$window"; then
        echo "error: private window $session:$window already exists" >&2
        exit 1
      fi
      pane=$(tmux new-window -dP -F '#{pane_id}' -t "$session:" -n "$window" -c "$workdir" "$codex_bin") \
        || { echo "error: failed to create private Codex window" >&2; exit 1; }
    else
      pane=$(tmux new-session -dP -F '#{pane_id}' -s "$session" -n "$window" -c "$workdir" "$codex_bin") \
        || { echo "error: failed to create private Codex session" >&2; exit 1; }
    fi
    tmux set-window-option -t "$pane" automatic-rename off 2>/dev/null || true
    tmux set-window-option -t "$pane" allow-rename off 2>/dev/null || true

    retries=${FM_TUI_CREATE_RETRIES:-30}
    sleep_s=${FM_TUI_CREATE_SLEEP:-0.1}
    case "$retries" in
      ''|*[!0-9]*|0) retries=30 ;;
    esac
    [ "$retries" -le 100 ] || retries=100
    [[ $sleep_s =~ ^(0|0\.[0-9]{1,3}|1(\.0{1,3})?)$ ]] || sleep_s=0.1
    created=
    i=0
    while [ "$i" -lt "$retries" ]; do
      record=$(tmux display-message -p -t "$pane" \
        '#{session_name}	#{window_name}	#{pane_index}	#{pane_current_command}	#{pane_current_path}' 2>/dev/null) || record=
      IFS=$'\t' read -r found_session found_window found_pane found_command found_project <<EOF
$record
EOF
      target="$found_session:$found_window.$found_pane"
      if [ "$found_session" = "$session" ] && [ "$found_window" = "$window" ] \
        && [ "$found_project" = "$workdir" ] && codex_command "$found_command" \
        && created=$(direct_record "$target"); then
        break
      fi
      created=
      i=$((i + 1))
      sleep "$sleep_s"
    done
    if [ -z "$created" ]; then
      tmux kill-window -t "$pane" 2>/dev/null || true
      echo "error: created private Codex pane did not become valid" >&2
      exit 1
    fi
    printf '%s\n' "$created"
    ;;
  *)
    echo "usage: fm-tui-direct.sh list | peek <target> <lines> | send <target> <message> | create <label> <directory>" >&2
    exit 2
    ;;
esac
