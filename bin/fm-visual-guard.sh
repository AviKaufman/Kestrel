#!/usr/bin/env bash
# Deterministic Codex GUI guard for Firstmate.
#
# Use this instead of plain GUI launchers, raw browser commands, or ad hoc
# Hyprland dispatches when Codex/crewmates need a visual surface.
# It keeps windows on workspace 99 on the hidden CODEX-HEADLESS output and keeps
# CODEX_GUI_GUARD=1 in the launched process environment so local wrappers can
# recognize Firstmate-owned GUI work.
#
# Usage:
#   fm-visual-guard.sh ensure
#   fm-visual-guard.sh exec -- <command ...>
#   fm-visual-guard.sh browser <url>
#   fm-visual-guard.sh screenshot <path>
#   fm-visual-guard.sh clients
#   fm-visual-guard.sh doctor
set -eu

VISUAL_OUTPUT=${FM_VISUAL_OUTPUT:-CODEX-HEADLESS}
VISUAL_WORKSPACE=${FM_VISUAL_WORKSPACE:-99}
VISUAL_GEOMETRY=${FM_VISUAL_GEOMETRY:-1440x1000@60,0x0,1}
VISUAL_CLASS=${FM_VISUAL_CLASS:-FirstmateVisual}
HYPRCTL=${FM_VISUAL_HYPRCTL:-hyprctl}
GRIM=${FM_VISUAL_GRIM:-grim}
JQ=${FM_VISUAL_JQ:-jq}
BROWSER_CMD=${FM_VISUAL_BROWSER_CMD:-}
ALLOW_USER_WORKSPACE=${FM_VISUAL_ALLOW_USER_WORKSPACE:-0}
LEGACY_VISUAL_CLASSES=${FM_VISUAL_LEGACY_CLASSES:-CodexVisual}
VERIFY_ATTEMPTS=${FM_VISUAL_VERIFY_ATTEMPTS:-10}
VERIFY_SLEEP=${FM_VISUAL_VERIFY_SLEEP:-0.2}
VERIFY_SETTLE_POLLS=${FM_VISUAL_VERIFY_SETTLE_POLLS:-3}
RECORD_SEPARATOR=$'\x1f'

usage() {
  cat <<EOF
Usage:
  $(basename "$0") ensure
  $(basename "$0") exec -- <command ...>
  $(basename "$0") browser <url>
  $(basename "$0") screenshot <path>
  $(basename "$0") clients
  $(basename "$0") doctor

Environment:
  FM_VISUAL_OUTPUT       output name, default CODEX-HEADLESS
  FM_VISUAL_WORKSPACE    workspace id/name, default 99
  FM_VISUAL_GEOMETRY     Hyprland monitor geometry, default 1440x1000@60,0x0,1
  FM_VISUAL_CLASS        browser class, default FirstmateVisual
  FM_VISUAL_HYPRCTL      hyprctl command/path, default hyprctl
  FM_VISUAL_GRIM         grim command/path, default grim
  FM_VISUAL_JQ           jq command/path, default jq
  FM_VISUAL_BROWSER_CMD  browser command/path, otherwise Chrome/Chromium on PATH
  FM_VISUAL_LEGACY_CLASSES  comma-separated older dedicated classes to corral, default CodexVisual
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

is_truthy() {
  case "$1" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

require_cmd() {
  local cmd=$1 label=$2
  command -v "$cmd" >/dev/null 2>&1 || die "missing required command: $label"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

hyprctl_call() {
  "$HYPRCTL" "$@"
}

jq_call() {
  "$JQ" "$@"
}

require_hyprland() {
  require_cmd "$HYPRCTL" hyprctl
  require_cmd "$JQ" jq
}

validate_visual_workspace() {
  case "$VISUAL_WORKSPACE" in
    ''|*[!0-9]*) return 0 ;;
    *)
      if [ "$VISUAL_WORKSPACE" -ge 1 ] && [ "$VISUAL_WORKSPACE" -le 11 ] && ! is_truthy "$ALLOW_USER_WORKSPACE"; then
        die "refusing to use user workspace '$VISUAL_WORKSPACE'; set FM_VISUAL_WORKSPACE to a non-user workspace such as 99"
      fi
      ;;
  esac
}

hyprland_json() {
  local what=$1 output
  if ! output=$(hyprctl_call "$what" -j 2>&1); then
    die "hyprctl $what -j failed: $output"
  fi
  printf '%s\n' "$output"
}

monitor_exists() {
  local monitors
  monitors=$(hyprland_json monitors)
  # shellcheck disable=SC2016 # jq receives $output via --arg, not shell expansion.
  printf '%s\n' "$monitors" | jq_call -e --arg output "$VISUAL_OUTPUT" \
    '.[] | select(.name == $output)' >/dev/null
}

ensure_visual_target() {
  local quiet=${1:-0}
  require_hyprland
  validate_visual_workspace

  if ! monitor_exists; then
    if ! hyprctl_call output create headless "$VISUAL_OUTPUT" >/dev/null 2>&1; then
      die "failed to create Hyprland headless output '$VISUAL_OUTPUT'"
    fi
  fi

  if ! monitor_exists; then
    die "Hyprland did not report headless output '$VISUAL_OUTPUT' after creation"
  fi

  if ! hyprctl_call keyword monitor "$VISUAL_OUTPUT,$VISUAL_GEOMETRY" >/dev/null 2>&1; then
    die "failed to configure monitor '$VISUAL_OUTPUT' with '$VISUAL_GEOMETRY'"
  fi

  if ! hyprctl_call keyword workspace "$VISUAL_WORKSPACE, monitor:$VISUAL_OUTPUT" >/dev/null 2>&1; then
    die "failed to bind workspace '$VISUAL_WORKSPACE' to '$VISUAL_OUTPUT'"
  fi

  if ! hyprctl_call dispatch moveworkspacetomonitor "$VISUAL_WORKSPACE" "$VISUAL_OUTPUT" >/dev/null 2>&1; then
    die "failed to move workspace '$VISUAL_WORKSPACE' to '$VISUAL_OUTPUT'"
  fi

  if [ "$quiet" != 1 ]; then
    printf 'ensured Codex visual target: output=%s workspace=%s\n' "$VISUAL_OUTPUT" "$VISUAL_WORKSPACE"
  fi
}

workspace_rule_status() {
  local rules
  rules=$(hyprctl_call workspacerules -j 2>/dev/null || true)
  [ -n "$rules" ] || return 1
  # shellcheck disable=SC2016 # jq receives $output and $workspace via --arg.
  printf '%s\n' "$rules" | jq_call -e \
    --arg output "$VISUAL_OUTPUT" \
    --arg workspace "$VISUAL_WORKSPACE" \
    '
      .[]
      | select(
          (.monitor? == $output)
          and (
            (.workspaceString? == $workspace)
            or (((.workspace? // "") | tostring) == $workspace)
          )
        )
    ' >/dev/null
}

shell_quote() {
  printf "'"
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
  printf "'"
}

join_shell_words() {
  local word out=""
  for word in "$@"; do
    out="${out}${out:+ }$(shell_quote "$word")"
  done
  printf '%s\n' "$out"
}

visual_browser_profile() {
  printf '%s\n' "${FM_VISUAL_BROWSER_PROFILE:-${XDG_STATE_HOME:-${HOME:-/tmp/.fm-visual}/.local/state}/firstmate-codex-visual-chrome}"
}

visual_devtools_profile() {
  printf '%s\n' "${FM_VISUAL_DEVTOOLS_PROFILE:-${XDG_STATE_HOME:-${HOME:-/tmp/.fm-visual}/.local/state}/firstmate-codex-devtools-chrome}"
}

devtools_chrome_args() {
  local args=${CHROME_DEVTOOLS_AXI_CHROME_ARGS:-}
  case " $args " in
    *" --class="*) ;;
    *) args="${args}${args:+ }--class=$VISUAL_CLASS" ;;
  esac
  case " $args " in
    *" --no-first-run "*) ;;
    *) args="${args}${args:+ }--no-first-run" ;;
  esac
  case " $args " in
    *" --no-default-browser-check "*) ;;
    *) args="${args}${args:+ }--no-default-browser-check" ;;
  esac
  printf '%s\n' "$args"
}

guard_env_assignments() {
  local browser_profile devtools_profile devtools_args
  browser_profile=$(visual_browser_profile)
  devtools_profile=$(visual_devtools_profile)
  devtools_args=$(devtools_chrome_args)

  printf 'CODEX_GUI_GUARD=1'
  printf ' CODEX_VISUAL_WORKSPACE=%s' "$(shell_quote "$VISUAL_WORKSPACE")"
  printf ' CODEX_VISUAL_OUTPUT=%s' "$(shell_quote "$VISUAL_OUTPUT")"
  printf ' CODEX_VISUAL_GEOMETRY=%s' "$(shell_quote "$VISUAL_GEOMETRY")"
  printf ' CODEX_VISUAL_CHROME_PROFILE=%s' "$(shell_quote "$browser_profile")"
  printf ' CHROME_DEVTOOLS_AXI_USER_DATA_DIR=%s' "$(shell_quote "$devtools_profile")"
  printf ' CHROME_DEVTOOLS_AXI_CHROME_ARGS=%s' "$(shell_quote "$devtools_args")"
}

class_matches_visual_identity() {
  local class=${1:-} initial=${2:-} legacy
  local -a legacy_classes
  [ "$class" = "$VISUAL_CLASS" ] || [ "$initial" = "$VISUAL_CLASS" ] && return 0
  IFS=',' read -r -a legacy_classes <<< "$LEGACY_VISUAL_CLASSES"
  for legacy in "${legacy_classes[@]}"; do
    [ -n "$legacy" ] || continue
    [ "$class" = "$legacy" ] || [ "$initial" = "$legacy" ] && return 0
  done
  return 1
}

client_cmdline() {
  local pid=${1:-}
  case "$pid" in
    ''|*[!0-9]*) return 0 ;;
  esac
  [ -r "/proc/$pid/cmdline" ] || return 0
  tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true
}

cmdline_matches_visual_profile() {
  local cmdline=$1 browser_profile devtools_profile
  browser_profile=$(visual_browser_profile)
  devtools_profile=$(visual_devtools_profile)
  case "$cmdline" in
    *"--user-data-dir=$browser_profile"*|*"--user-data-dir=$devtools_profile"*|*"--userDataDir=$devtools_profile"*)
      return 0
      ;;
  esac
  return 1
}

client_rows() {
  local clients monitors
  clients=$(hyprland_json clients)
  monitors=$(hyprland_json monitors)
  # shellcheck disable=SC2016
  printf '%s\n' "$clients" | jq_call -r --argjson monitors "$monitors" '
    .[] as $client
    | (($client.monitor // "") | tostring) as $monitor
    | (($monitors | map(select(((.id? // "") | tostring) == $monitor) | .name) | first) // $monitor) as $monitor_name
    | [
        ($client.address // ""),
        (($client.pid? // "") | tostring),
        ($client.class // ""),
        ($client.initialClass // ""),
        ($client.title // ""),
        (($client.workspace.id? // "") | tostring),
        ($client.workspace.name // ""),
        $monitor_name
      ]
    | @tsv
    | gsub("\t"; "\u001f")
  '
}

visual_client_rows() {
  local address pid class initial title workspace_id workspace_name monitor cmdline
  client_rows | while IFS="$RECORD_SEPARATOR" read -r address pid class initial title workspace_id workspace_name monitor; do
    [ -n "$address" ] || continue
    cmdline=$(client_cmdline "$pid")
    if class_matches_visual_identity "$class" "$initial" || cmdline_matches_visual_profile "$cmdline"; then
      printf '%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
        "$address" "$RECORD_SEPARATOR" "$pid" "$RECORD_SEPARATOR" "${class:-$initial}" "$RECORD_SEPARATOR" \
        "$title" "$RECORD_SEPARATOR" "$workspace_id" "$RECORD_SEPARATOR" "$workspace_name" "$RECORD_SEPARATOR" "$monitor"
    fi
  done
}

visual_client_addresses() {
  local address pid class title workspace_id workspace_name monitor
  visual_client_rows | while IFS="$RECORD_SEPARATOR" read -r address pid class title workspace_id workspace_name monitor; do
    [ -n "$address" ] || continue
    printf '%s\n' "$address"
  done
}

client_address_in_set() {
  local needle=$1 addresses=$2 address
  while IFS= read -r address; do
    [ "$address" = "$needle" ] && return 0
  done <<EOF
$addresses
EOF
  return 1
}

visual_client_is_hidden() {
  local workspace_id=$1 workspace_name=$2 monitor=$3
  { [ "$workspace_id" = "$VISUAL_WORKSPACE" ] || [ "$workspace_name" = "$VISUAL_WORKSPACE" ]; } \
    && [ "$monitor" = "$VISUAL_OUTPUT" ]
}

visual_client_status() {
  local preexisting=${1:-} misplaced=0 new_count=0
  local address pid class title workspace_id workspace_name monitor rows
  rows=$(visual_client_rows)
  if [ -n "$rows" ]; then
    while IFS="$RECORD_SEPARATOR" read -r address pid class title workspace_id workspace_name monitor; do
      [ -n "$address" ] || continue
      if ! client_address_in_set "$address" "$preexisting"; then
        new_count=$((new_count + 1))
      fi
      if ! visual_client_is_hidden "$workspace_id" "$workspace_name" "$monitor"; then
        misplaced=$((misplaced + 1))
      fi
    done <<EOF
$rows
EOF
  fi
  printf '%s%s%s\n' "$misplaced" "$RECORD_SEPARATOR" "$new_count"
}

remediate_misplaced_visual_clients() {
  local rows address pid class title workspace_id workspace_name monitor moved=0
  rows=$(visual_client_rows)
  while IFS="$RECORD_SEPARATOR" read -r address pid class title workspace_id workspace_name monitor; do
    [ -n "$address" ] || continue
    visual_client_is_hidden "$workspace_id" "$workspace_name" "$monitor" && continue
    if ! hyprctl_call dispatch movetoworkspacesilent "$VISUAL_WORKSPACE,address:$address" >/dev/null 2>&1; then
      die "failed to move Codex visual client $address from workspace '${workspace_name:-$workspace_id}' to '$VISUAL_WORKSPACE'"
    fi
    moved=$((moved + 1))
    warn "moved Codex visual client $address pid=$pid class=$class from workspace '${workspace_name:-$workspace_id}' on $monitor to '$VISUAL_WORKSPACE'"
  done <<EOF
$rows
EOF
  printf '%s\n' "$moved"
}

verify_visual_placement() {
  local preexisting=${1:-} remaining=$VERIFY_ATTEMPTS status misplaced new_count remediated stable_polls=0 saw_new_client=0
  while [ "$remaining" -gt 0 ]; do
    remediated=$(remediate_misplaced_visual_clients)
    status=$(visual_client_status "$preexisting")
    IFS="$RECORD_SEPARATOR" read -r misplaced new_count <<EOF
$status
EOF
    if [ "$new_count" -gt 0 ]; then
      saw_new_client=1
    fi
    if [ "$new_count" -gt 0 ] && [ "$misplaced" -eq 0 ] && [ "$remediated" -eq 0 ]; then
      stable_polls=$((stable_polls + 1))
      if [ "$stable_polls" -ge "$VERIFY_SETTLE_POLLS" ]; then
        return 0
      fi
    else
      stable_polls=0
    fi
    sleep "$VERIFY_SLEEP"
    remaining=$((remaining - 1))
  done
  status=$(visual_client_status "$preexisting")
  IFS="$RECORD_SEPARATOR" read -r misplaced new_count <<EOF
$status
EOF
  if [ "$new_count" -gt 0 ]; then
    saw_new_client=1
  fi
  [ "${misplaced:-0}" -eq 0 ] || die "Codex visual client remains outside workspace $VISUAL_WORKSPACE on $VISUAL_OUTPUT"
  [ "$saw_new_client" -eq 0 ] || die "Codex visual client did not remain hidden for $VERIFY_SETTLE_POLLS consecutive polls before verification timed out"
}

guard_exec() {
  local command_string dispatch_string env_assignments preexisting
  [ "${1:-}" = "--" ] && shift
  [ "$#" -gt 0 ] || die "exec requires a command after --"

  ensure_visual_target 1
  preexisting=$(visual_client_addresses)
  command_string=$(join_shell_words "$@")
  env_assignments=$(guard_env_assignments)
  dispatch_string="[workspace $VISUAL_WORKSPACE silent] env $env_assignments $command_string"
  if ! hyprctl_call dispatch exec "$dispatch_string" >/dev/null 2>&1; then
    die "Hyprland failed to dispatch command to workspace '$VISUAL_WORKSPACE'"
  fi
  verify_visual_placement "$preexisting"
  printf 'launched on workspace %s: %s\n' "$VISUAL_WORKSPACE" "$command_string"
}

find_browser_command() {
  local candidate
  if [ -n "$BROWSER_CMD" ]; then
    if have_cmd "$BROWSER_CMD"; then
      printf '%s\n' "$BROWSER_CMD"
      return 0
    fi
    return 1
  fi
  for candidate in google-chrome-stable google-chrome chromium chromium-browser; do
    if have_cmd "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

guard_browser() {
  local url=${1:-} browser profile
  [ -n "$url" ] || die "browser requires a URL"
  browser=$(find_browser_command) || die "no Chrome/Chromium browser command found"

  profile=$(visual_browser_profile)
  guard_exec -- "$browser" \
    "--class=$VISUAL_CLASS" \
    "--user-data-dir=$profile" \
    --no-first-run \
    --no-default-browser-check \
    --new-window \
    "$url"
}

guard_screenshot() {
  local path=${1:-} parent
  [ -n "$path" ] || die "screenshot requires an output path"
  ensure_visual_target 1
  require_cmd "$GRIM" grim
  case "$path" in
    */*)
      parent=${path%/*}
      [ -d "$parent" ] || die "screenshot parent directory does not exist: $parent"
      ;;
  esac
  if ! "$GRIM" -o "$VISUAL_OUTPUT" "$path"; then
    die "grim failed to capture output '$VISUAL_OUTPUT'"
  fi
  printf 'screenshot captured from %s: %s\n' "$VISUAL_OUTPUT" "$path"
}

guard_clients() {
  local rows address pid class initial title workspace_id workspace_name monitor cmdline
  require_hyprland
  rows=$(client_rows | while IFS="$RECORD_SEPARATOR" read -r address pid class initial title workspace_id workspace_name monitor; do
    [ -n "$address" ] || continue
    cmdline=$(client_cmdline "$pid")
    if [ "$monitor" = "$VISUAL_OUTPUT" ] \
      || [ "$workspace_name" = "$VISUAL_WORKSPACE" ] \
      || [ "$workspace_id" = "$VISUAL_WORKSPACE" ] \
      || class_matches_visual_identity "$class" "$initial" \
      || cmdline_matches_visual_profile "$cmdline"; then
      printf '%s\t%s\t%s\t%s\t%s\n' \
        "$address" "${class:-$initial}" "$title" "${workspace_name:-$workspace_id}" "$monitor"
    fi
  done)
  if [ -n "$rows" ]; then
    printf '%s\n' "$rows"
  else
    printf '(none)\n'
  fi
}

guard_doctor() {
  local failures=0 browser

  if have_cmd "$HYPRCTL"; then
    printf 'ok: hyprctl\n'
  else
    printf 'error: missing required command: hyprctl\n' >&2
    failures=$((failures + 1))
  fi

  if have_cmd "$JQ"; then
    printf 'ok: jq\n'
  else
    printf 'error: missing required command: jq\n' >&2
    failures=$((failures + 1))
  fi

  if have_cmd "$GRIM"; then
    printf 'ok: grim\n'
  else
    printf 'error: missing required command: grim\n' >&2
    failures=$((failures + 1))
  fi

  if browser=$(find_browser_command); then
    printf 'ok: browser=%s\n' "$browser"
  else
    warn "no Chrome/Chromium browser command found; browser subcommand will fail"
  fi

  [ "$failures" -eq 0 ] || return 1
  ensure_visual_target 1
  printf 'ok: visual target output=%s workspace=%s geometry=%s class=%s\n' \
    "$VISUAL_OUTPUT" "$VISUAL_WORKSPACE" "$VISUAL_GEOMETRY" "$VISUAL_CLASS"
  if workspace_rule_status; then
    printf 'ok: workspace rule %s -> %s\n' "$VISUAL_WORKSPACE" "$VISUAL_OUTPUT"
  else
    warn "workspace rule for $VISUAL_WORKSPACE -> $VISUAL_OUTPUT not visible through hyprctl workspacerules"
  fi
  printf 'clients:\n'
  guard_clients
}

main() {
  local cmd=${1:-}
  [ -n "$cmd" ] || { usage; exit 2; }
  shift || true

  case "$cmd" in
    ensure) ensure_visual_target ;;
    exec) guard_exec "$@" ;;
    browser) guard_browser "$@" ;;
    screenshot) guard_screenshot "$@" ;;
    clients) guard_clients ;;
    doctor) guard_doctor ;;
    -h|--help|help) usage ;;
    *) usage >&2; exit 2 ;;
  esac
}

main "$@"
