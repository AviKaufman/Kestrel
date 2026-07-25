#!/usr/bin/env bash
# fm-supervisor-target-lib.sh - the single owner of supervisor-pane discovery.
#
# The away-mode daemon (bin/fm-supervise-daemon.sh) must know which pane runs
# firstmate itself, both to inject escalations into it and, for the daemon, to
# validate that target at startup. The script-owned away launcher
# (bin/fm-afk-launch.sh) must resolve the SAME captain pane BEFORE it creates a
# separate, non-visible terminal for the daemon, so it can pass that pane in as
# FM_SUPERVISOR_TARGET (otherwise the daemon, running in its own terminal, would
# auto-discover its OWN pane and inject there instead of into the captain's).
#
# Because both callers need the identical resolution, it lives here once. The
# function names and precedence are unchanged from when this logic lived inline
# in bin/fm-supervise-daemon.sh, so its unit tests (tests/fm-daemon.test.sh)
# keep exercising the same names after the daemon sources this file.

# shellcheck source=bin/fm-session-lock-lib.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/fm-session-lock-lib.sh"

# Default supervisor pane target/backend when nothing is configured or detected.
# "firstmate:0" is a tmux session:window name, so the bare fallback (nothing
# configured, nothing detected) assumes tmux - matching the daemon's pre-herdr
# behavior byte-for-byte when run outside both tmux and herdr.
FM_SUPERVISOR_TARGET_DEFAULT="firstmate:0"
FM_SUPERVISOR_BACKEND_DEFAULT="tmux"

# discover_supervisor_target: resolve the pane running firstmate. Priority:
#   1. FM_SUPERVISOR_TARGET env (explicit override) - may be a tmux target or a
#      herdr "<session>:<pane-id>" target (paired with discover_supervisor_backend
#      to know which).
#   2. $TMUX_PANE - tmux sets this in every pane's environment; inherited by a
#      process launched from firstmate's own pane.
#   3. $HERDR_ENV=1 + $HERDR_PANE_ID - herdr injects both into every process it
#      manages a pane for; compose the "<session>:<pane-id>" target from
#      $HERDR_SESSION (defaulting to "default", mirroring bin/backends/herdr.sh's
#      fm_backend_herdr_session) and $HERDR_PANE_ID. Checked after $TMUX_PANE so a
#      tmux pane nested inside herdr still resolves to tmux, matching
#      fm_backend_detect's innermost-first rule.
#   4. FM_SUPERVISOR_TARGET_DEFAULT - legacy tmux fallback (may not resolve if the
#      session is named differently). Returns 1 so the caller can warn.
discover_supervisor_target() {
  if [ -n "${FM_SUPERVISOR_TARGET:-}" ]; then
    printf '%s' "$FM_SUPERVISOR_TARGET"
    return 0
  fi
  if [ -n "${TMUX_PANE:-}" ]; then
    printf '%s' "$TMUX_PANE"
    return 0
  fi
  if [ "${HERDR_ENV:-}" = "1" ] && [ -n "${HERDR_PANE_ID:-}" ]; then
    printf '%s:%s' "${HERDR_SESSION:-default}" "$HERDR_PANE_ID"
    return 0
  fi
  printf '%s' "$FM_SUPERVISOR_TARGET_DEFAULT"
  return 1
}

# discover_supervisor_backend: resolve the supervisor pane's BACKEND, independent
# of the target string so an explicit FM_SUPERVISOR_TARGET override still knows
# which primitives (tmux vs herdr) to dispatch through. Priority mirrors
# discover_supervisor_target and bin/fm-backend.sh's fm_backend_detect:
#   1. FM_SUPERVISOR_BACKEND env (explicit override).
#   2. $TMUX_PANE set - tmux.
#   3. $HERDR_ENV=1 (with $HERDR_PANE_ID present) - herdr.
#   4. FM_SUPERVISOR_BACKEND_DEFAULT (tmux) - matches the target fallback. Returns 1.
discover_supervisor_backend() {
  if [ -n "${FM_SUPERVISOR_BACKEND:-}" ]; then
    printf '%s' "$FM_SUPERVISOR_BACKEND"
    return 0
  fi
  if [ -n "${TMUX_PANE:-}" ]; then
    printf 'tmux'
    return 0
  fi
  if [ "${HERDR_ENV:-}" = "1" ] && [ -n "${HERDR_PANE_ID:-}" ]; then
    printf 'herdr'
    return 0
  fi
  printf '%s' "$FM_SUPERVISOR_BACKEND_DEFAULT"
  return 1
}

# Print one environment value from the verified lock-owning process.
# Linux /proc is the only accepted fallback source: if it is unavailable, the
# caller fails closed and may still use the normal inherited-session markers.
fm_supervisor_locked_process_env() {  # <pid> <key>
  local pid=$1 key=$2 record
  case "$key" in
    FM_SUPERVISOR_TARGET|FM_SUPERVISOR_BACKEND|TMUX_PANE|HERDR_ENV|HERDR_PANE_ID|HERDR_SESSION) ;;
    *) return 1 ;;
  esac
  [ -r "/proc/$pid/environ" ] || return 1
  record=$(tr '\000' '\n' < "/proc/$pid/environ" | grep -m1 "^${key}=") || return 1
  printf '%s' "${record#*=}"
}

# Resolve the active primary session from this home's verified live session
# lock, never from a tmux inventory scan. Prints backend<TAB>target.
discover_locked_supervisor_context() {  # <state-dir>
  local state=$1 lock_pid explicit_target explicit_backend tmux_pane
  local herdr_env herdr_pane herdr_session target backend
  [ -f "$state/.lock" ] && [ ! -L "$state/.lock" ] || return 1
  lock_pid=$(cat "$state/.lock" 2>/dev/null) || return 1
  case "$lock_pid" in
    ''|*[!0-9]*|1) return 1 ;;
  esac
  fm_harness_pid_alive "$lock_pid" || return 1

  explicit_target=$(fm_supervisor_locked_process_env "$lock_pid" FM_SUPERVISOR_TARGET 2>/dev/null || true)
  explicit_backend=$(fm_supervisor_locked_process_env "$lock_pid" FM_SUPERVISOR_BACKEND 2>/dev/null || true)
  tmux_pane=$(fm_supervisor_locked_process_env "$lock_pid" TMUX_PANE 2>/dev/null || true)
  herdr_env=$(fm_supervisor_locked_process_env "$lock_pid" HERDR_ENV 2>/dev/null || true)
  herdr_pane=$(fm_supervisor_locked_process_env "$lock_pid" HERDR_PANE_ID 2>/dev/null || true)
  herdr_session=$(fm_supervisor_locked_process_env "$lock_pid" HERDR_SESSION 2>/dev/null || true)

  if [ -n "$explicit_target" ]; then
    target=$explicit_target
  elif [ -n "$tmux_pane" ]; then
    target=$tmux_pane
  elif [ "$herdr_env" = 1 ] && [ -n "$herdr_pane" ]; then
    target="${herdr_session:-default}:$herdr_pane"
  else
    return 1
  fi

  if [ -n "$explicit_backend" ]; then
    backend=$explicit_backend
  elif [ -n "$tmux_pane" ]; then
    backend=tmux
  elif [ "$herdr_env" = 1 ] && [ -n "$herdr_pane" ]; then
    backend=herdr
  else
    return 1
  fi
  printf '%s\t%s\n' "$backend" "$target"
}
