#!/usr/bin/env bash
# Print the recovery-grade agent state for one Firstmate-managed task.
# Usage: fm-tui-agent-state.sh <task-id>
#
# This is a narrow adapter for fm-tui.
# bin/fm-backend.sh remains the owner of endpoint resolution and liveness.
set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -z "${FM_HOME:-}" ]; then
  echo "error: FM_HOME is required" >&2
  exit 1
fi
STATE="${FM_STATE_OVERRIDE:-$FM_HOME/state}"
ID=${1:-}
case "$ID" in
  ''|[!A-Za-z0-9]*|*[!A-Za-z0-9._-]*)
    echo "error: invalid task id '$ID'" >&2
    exit 1
    ;;
esac
META="$STATE/$ID.meta"
if [ ! -f "$META" ]; then
  echo "error: no metadata for task '$ID'" >&2
  exit 1
fi

# shellcheck source=bin/fm-backend.sh
. "$SCRIPT_DIR/fm-backend.sh"

backend=$(fm_backend_of_meta "$META")
target=$(fm_backend_target_of_meta "$META")
if [ -z "$target" ]; then
  printf 'missing\n'
  exit 0
fi
fm_backend_agent_state "$backend" "$target"
printf '\n'
