#!/usr/bin/env bash
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP_ROOT=$(mktemp -d)
LOCK_HOLDER_PID=
cleanup() {
  if [ -n "$LOCK_HOLDER_PID" ]; then
    kill "$LOCK_HOLDER_PID" 2>/dev/null || true
    wait "$LOCK_HOLDER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

HOME_DIR="$TMP_ROOT/home"
FAKEBIN="$TMP_ROOT/bin"
LOG="$TMP_ROOT/tmux.log"
mkdir -p "$HOME_DIR/state" "$FAKEBIN"
: > "$LOG"

cat > "$HOME_DIR/state/managed.meta" <<'EOF'
window=fleet:fm-managed
harness=codex
kind=ship
EOF

cat > "$FAKEBIN/tmux" <<'EOF'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$FM_TMUX_LOG"
case "$1" in
  has-session) exit 0 ;;
  list-panes)
    printf 'fleet\tfm-managed\t0\tcodex\t/worktrees/managed\n'
    printf 'private\tnotes\t0\tcodex\t/projects/notes\n'
    printf 'private\tshell\t0\tzsh\t/projects/shell\n'
    ;;
  list-windows)
    printf 'fm-managed\n'
    ;;
  display-message)
    target=
    format=${*: -1}
    while [ "$#" -gt 0 ]; do
      if [ "$1" = -t ]; then target=$2; shift 2; continue; fi
      shift
    done
    case "$format" in
      *session_name*)
        case "$target" in
          private:notes.0) printf 'private\tnotes\t0\tcodex\t/projects/notes\n' ;;
          private:codex-notes.0|%7)
            command=codex
            [ "${FM_BAD_CREATE:-0}" != 1 ] || command=zsh
            printf 'private\tcodex-notes\t0\t%s\t%s\n' "$command" "$FM_PRIVATE_TEST_CWD"
            ;;
          fleet:fm-managed.0) printf 'fleet\tfm-managed\t0\tcodex\t/worktrees/managed\n' ;;
          *) exit 1 ;;
        esac
        ;;
      '#S') printf 'private\n' ;;
      *cursor_y*) printf '0\n' ;;
      *pane_current_command*) printf 'codex\n' ;;
      *pane_id*)
        [ "$target" != "%gone" ] || exit 1
        printf '%%1\n'
        ;;
    esac
    ;;
  new-window)
    [ "${FM_FAIL_CREATE:-0}" != 1 ] || exit 1
    printf '%%7\n'
    ;;
  set-window-option) ;;
  kill-window) ;;
  capture-pane)
    case " $* " in
      *" -e "*) printf '›\n' ;;
      *) printf 'bounded direct capture\n' ;;
    esac
    ;;
  send-keys) ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$FAKEBIN/tmux" "$ROOT/bin/fm-tui-agent-state.sh" "$ROOT/bin/fm-tui-direct.sh" "$ROOT/bin/fm-tui-hub.sh"
cat > "$FAKEBIN/codex" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$FAKEBIN/codex"

common_env=(
  "PATH=$FAKEBIN:$PATH"
  "FM_HOME=$HOME_DIR"
  "FM_STATE_OVERRIDE=$HOME_DIR/state"
  "FM_ROOT_OVERRIDE=$ROOT"
  "FM_TMUX_LOG=$LOG"
  "FM_SEND_SLEEP=0"
)

state=$(env "${common_env[@]}" "$ROOT/bin/fm-tui-agent-state.sh" managed)
[ "$state" = alive ] || fail "managed agent state = '$state', want alive"

listed=$(env "${common_env[@]}" "$ROOT/bin/fm-tui-direct.sh" list)
[ "$listed" = $'private:notes.0\t/projects/notes' ] \
  || fail "direct list did not include only the unowned Codex pane: '$listed'"

capture=$(env "${common_env[@]}" "$ROOT/bin/fm-tui-direct.sh" peek private:notes.0 20)
[ "$capture" = "bounded direct capture" ] || fail "direct capture = '$capture'"

if env "${common_env[@]}" "$ROOT/bin/fm-tui-direct.sh" peek fleet:fm-managed.0 20 >/dev/null 2>&1; then
  fail "direct adapter accepted a Firstmate-managed pane"
fi
if env "${common_env[@]}" "$ROOT/bin/fm-tui-direct.sh" peek private:notes.bad 20 >/dev/null 2>&1; then
  fail "direct adapter accepted an invalid target grammar"
fi

# shellcheck disable=SC2016 # Literal shell syntax proves argument-array safety.
message='literal $HOME; $(touch /tmp/must-not-run)'
sent=$(env "${common_env[@]}" "$ROOT/bin/fm-tui-direct.sh" send private:notes.0 "$message")
[ "$sent" = sent ] || fail "direct send = '$sent'"
grep -F -- "send-keys -t private:notes.0 -l $message" "$LOG" >/dev/null \
  || fail "direct send did not preserve the literal message argument"

hub=$(env "${common_env[@]}" FM_SUPERVISOR_BACKEND=tmux FM_SUPERVISOR_TARGET=%9 \
  "$ROOT/bin/fm-tui-hub.sh" resolve)
[ "$hub" = $'tmux\t%9' ] || fail "hub resolve = '$hub'"

hub_sent=$(env "${common_env[@]}" FM_SUPERVISOR_BACKEND=tmux FM_SUPERVISOR_TARGET=%9 \
  "$ROOT/bin/fm-tui-hub.sh" send tmux %9 "$message")
[ "$hub_sent" = sent ] || fail "hub send = '$hub_sent'"
grep -F -- "send-keys -t %9 -l $message" "$LOG" >/dev/null \
  || fail "hub send did not preserve the literal message argument"

env TMUX_PANE=%9 bash -c 'exec -a codex sleep 60' &
LOCK_HOLDER_PID=$!
printf '%s\n' "$LOCK_HOLDER_PID" > "$HOME_DIR/state/.lock"
locked_hub=$(env "${common_env[@]}" FM_SUPERVISOR_BACKEND= FM_SUPERVISOR_TARGET= \
  TMUX_PANE= HERDR_ENV= HERDR_PANE_ID= "$ROOT/bin/fm-tui-hub.sh" resolve)
[ "$locked_hub" = $'tmux\t%9' ] \
  || fail "hub did not resolve the active lock-owning primary context: '$locked_hub'"
kill "$LOCK_HOLDER_PID"
wait "$LOCK_HOLDER_PID" 2>/dev/null || true
LOCK_HOLDER_PID=
rm -f "$HOME_DIR/state/.lock"

if env "${common_env[@]}" FM_SUPERVISOR_BACKEND=tmux FM_SUPERVISOR_TARGET=%9 \
  "$ROOT/bin/fm-tui-hub.sh" send tmux %8 hello >/dev/null 2>&1; then
  fail "hub adapter did not revalidate a changed target"
fi
if env "${common_env[@]}" FM_SUPERVISOR_BACKEND=tmux FM_SUPERVISOR_TARGET=%gone \
  "$ROOT/bin/fm-tui-hub.sh" resolve >/dev/null 2>&1; then
  fail "hub adapter accepted a missing supervisor target"
fi
if env "${common_env[@]}" FM_SUPERVISOR_BACKEND= FM_SUPERVISOR_TARGET= TMUX_PANE= HERDR_ENV= \
  "$ROOT/bin/fm-tui-hub.sh" resolve >/dev/null 2>&1; then
  fail "hub adapter accepted the ambiguous legacy fallback"
fi

PRIVATE_CWD="$TMP_ROOT/private-project"
mkdir -p "$PRIVATE_CWD"
meta_count_before=$(find "$HOME_DIR/state" -name '*.meta' -type f | wc -l)
created=$(env "${common_env[@]}" TMUX=/tmp/fake FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" \
  "$ROOT/bin/fm-tui-direct.sh" create notes "$PRIVATE_CWD")
[ "$created" = "private:codex-notes.0	$PRIVATE_CWD" ] \
  || fail "private create = '$created'"
meta_count_after=$(find "$HOME_DIR/state" -name '*.meta' -type f | wc -l)
[ "$meta_count_after" -eq "$meta_count_before" ] \
  || fail "private create wrote Firstmate metadata"
grep -F -- "new-window -dP -F #{pane_id} -t private: -n codex-notes -c $PRIVATE_CWD" "$LOG" >/dev/null \
  || fail "private create did not use the expected tmux argv"

if env "${common_env[@]}" TMUX=/tmp/fake FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" \
  "$ROOT/bin/fm-tui-direct.sh" create 'bad;label' "$PRIVATE_CWD" >/dev/null 2>&1; then
  fail "private create accepted an unsafe label"
fi
if env "${common_env[@]}" TMUX=/tmp/fake FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" \
  "$ROOT/bin/fm-tui-direct.sh" create notes relative/path >/dev/null 2>&1; then
  fail "private create accepted a relative path"
fi
UNSAFE_BIN="$TMP_ROOT/unsafe;bin"
mkdir -p "$UNSAFE_BIN"
cp "$FAKEBIN/codex" "$UNSAFE_BIN/codex"
if env "${common_env[@]}" PATH="$UNSAFE_BIN:$FAKEBIN:$PATH" TMUX=/tmp/fake \
  FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" \
  "$ROOT/bin/fm-tui-direct.sh" create notes "$PRIVATE_CWD" >/dev/null 2>&1; then
  fail "private create accepted a shell-sensitive Codex command path"
fi
if env "${common_env[@]}" TMUX=/tmp/fake FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" FM_FAIL_CREATE=1 \
  "$ROOT/bin/fm-tui-direct.sh" create notes "$PRIVATE_CWD" >/dev/null 2>&1; then
  fail "private create ignored a tmux launch failure"
fi
if env "${common_env[@]}" TMUX=/tmp/fake FM_PRIVATE_TEST_CWD="$PRIVATE_CWD" FM_BAD_CREATE=1 \
  FM_TUI_CREATE_RETRIES=1 FM_TUI_CREATE_SLEEP=0 \
  "$ROOT/bin/fm-tui-direct.sh" create notes "$PRIVATE_CWD" >/dev/null 2>&1; then
  fail "private create exposed a pane that failed Codex revalidation"
fi
grep -F -- "kill-window -t %7" "$LOG" >/dev/null \
  || fail "private create did not roll back a pane that failed revalidation"

echo "PASS: fm-tui adapters enforce managed ownership and private Codex routing"
