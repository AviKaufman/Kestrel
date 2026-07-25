#!/usr/bin/env bash
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "$TMP_ROOT"' EXIT

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
          fleet:fm-managed.0) printf 'fleet\tfm-managed\t0\tcodex\t/worktrees/managed\n' ;;
          *) exit 1 ;;
        esac
        ;;
      *cursor_y*) printf '0\n' ;;
      *pane_current_command*) printf 'codex\n' ;;
      *pane_id*)
        [ "$target" != "%gone" ] || exit 1
        printf '%%1\n'
        ;;
    esac
    ;;
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

echo "PASS: fm-tui adapters enforce managed ownership and private Codex routing"
