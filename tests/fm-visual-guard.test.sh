#!/usr/bin/env bash
# Behavior tests for bin/fm-visual-guard.sh.
set -u

# shellcheck source=tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

TMP_ROOT=$(fm_test_tmproot fm-visual-guard)
FAKEBIN=$(fm_fakebin "$TMP_ROOT")
LOG="$TMP_ROOT/calls.log"
MONITOR_STATE="$TMP_ROOT/monitor-created"
LEAK_STATE="$TMP_ROOT/leak-client"
CLIENT_COUNT_STATE="$TMP_ROOT/client-count"

write_fake_tools() {
  cat > "$FAKEBIN/hyprctl" <<'SH'
#!/usr/bin/env bash
set -u
printf 'hyprctl' >> "$FM_VISUAL_TEST_LOG"
for arg in "$@"; do
  printf '\t%s' "$arg" >> "$FM_VISUAL_TEST_LOG"
done
printf '\n' >> "$FM_VISUAL_TEST_LOG"

if [ "${1:-}" = "monitors" ] && [ "${2:-}" = "-j" ]; then
  if [ -e "$FM_VISUAL_TEST_MONITOR_STATE" ]; then
    printf '[{"name":"CODEX-HEADLESS"}]\n'
  else
    printf '[{"name":"eDP-1"}]\n'
  fi
  exit 0
fi

if [ "${1:-}" = "clients" ] && [ "${2:-}" = "-j" ]; then
  count=0
  [ -f "${FM_VISUAL_TEST_CLIENT_COUNT_STATE:-}" ] && count=$(cat "$FM_VISUAL_TEST_CLIENT_COUNT_STATE")
  count=$((count + 1))
  printf '%s\n' "$count" > "$FM_VISUAL_TEST_CLIENT_COUNT_STATE"

  if [ "${FM_VISUAL_TEST_CLIENT_MODE:-}" = "late-leak" ]; then
    if [ "$count" -lt 3 ]; then
      cat <<'JSON'
[
  {"address":"0xdef","pid":12347,"class":"google-chrome","initialClass":"google-chrome","title":"User Chrome","workspace":{"id":1,"name":"1"},"monitor":"eDP-1"}
]
JSON
    elif [ -e "${FM_VISUAL_TEST_LEAK_STATE:-}" ]; then
      cat <<'JSON'
[
  {"address":"0xced","pid":12346,"class":"CodexVisual","initialClass":"CodexVisual","title":"Legacy","workspace":{"id":11,"name":"11"},"monitor":"eDP-1"},
  {"address":"0xdef","pid":12347,"class":"google-chrome","initialClass":"google-chrome","title":"User Chrome","workspace":{"id":1,"name":"1"},"monitor":"eDP-1"}
]
JSON
    else
      cat <<'JSON'
[
  {"address":"0xced","pid":12346,"class":"CodexVisual","initialClass":"CodexVisual","title":"Legacy","workspace":{"id":99,"name":"99"},"monitor":"CODEX-HEADLESS"},
  {"address":"0xdef","pid":12347,"class":"google-chrome","initialClass":"google-chrome","title":"User Chrome","workspace":{"id":1,"name":"1"},"monitor":"eDP-1"}
]
JSON
    fi
  elif [ -e "${FM_VISUAL_TEST_LEAK_STATE:-}" ]; then
    cat <<'JSON'
[
  {"address":"0xabc","pid":12345,"class":"FirstmateVisual","initialClass":"FirstmateVisual","title":"Preview","workspace":{"id":99,"name":"99"},"monitor":"CODEX-HEADLESS"},
  {"address":"0xced","pid":12346,"class":"CodexVisual","initialClass":"CodexVisual","title":"Legacy","workspace":{"id":11,"name":"11"},"monitor":"eDP-1"},
  {"address":"0xdef","pid":12347,"class":"google-chrome","initialClass":"google-chrome","title":"User Chrome","workspace":{"id":1,"name":"1"},"monitor":"eDP-1"}
]
JSON
  else
    cat <<'JSON'
[
  {"address":"0xabc","pid":12345,"class":"FirstmateVisual","initialClass":"FirstmateVisual","title":"Preview","workspace":{"id":99,"name":"99"},"monitor":"CODEX-HEADLESS"},
  {"address":"0xdef","pid":12347,"class":"google-chrome","initialClass":"google-chrome","title":"User Chrome","workspace":{"id":1,"name":"1"},"monitor":"eDP-1"}
]
JSON
  fi
  exit 0
fi

if [ "${1:-}" = "dispatch" ] && [ "${2:-}" = "movetoworkspacesilent" ]; then
  rm -f "${FM_VISUAL_TEST_LEAK_STATE:-}"
  exit 0
fi

if [ "${1:-}" = "output" ] && [ "${2:-}" = "create" ]; then
  : > "$FM_VISUAL_TEST_MONITOR_STATE"
  exit 0
fi

exit 0
SH
  chmod +x "$FAKEBIN/hyprctl"

  cat > "$FAKEBIN/grim" <<'SH'
#!/usr/bin/env bash
set -u
printf 'grim' >> "$FM_VISUAL_TEST_LOG"
for arg in "$@"; do
  printf '\t%s' "$arg" >> "$FM_VISUAL_TEST_LOG"
done
printf '\n' >> "$FM_VISUAL_TEST_LOG"
printf 'fake png\n' > "${@: -1}"
SH
  chmod +x "$FAKEBIN/grim"

  cat > "$FAKEBIN/google-chrome-stable" <<'SH'
#!/usr/bin/env bash
exit 0
SH
  chmod +x "$FAKEBIN/google-chrome-stable"
}

run_guard() {
    FM_VISUAL_TEST_LOG="$LOG" \
    FM_VISUAL_TEST_MONITOR_STATE="$MONITOR_STATE" \
    FM_VISUAL_TEST_LEAK_STATE="$LEAK_STATE" \
    FM_VISUAL_TEST_CLIENT_COUNT_STATE="$CLIENT_COUNT_STATE" \
    FM_VISUAL_HYPRCTL=hyprctl \
    FM_VISUAL_GRIM=grim \
    FM_VISUAL_VERIFY_SLEEP=0 \
    PATH="$FAKEBIN:$PATH" \
    "$ROOT/bin/fm-visual-guard.sh" "$@"
}

test_exec_preserves_arguments_and_silent_workspace() {
  local output code
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  output=$(run_guard exec -- printf "two words" "quote's" "\$dollar" 2>&1)
  code=$?
  expect_code 0 "$code" "exec guard"
  assert_grep $'hyprctl\toutput\tcreate\theadless\tCODEX-HEADLESS' "$LOG" \
    "ensure did not create missing CODEX-HEADLESS output"
  assert_grep $'hyprctl\tdispatch\texec\t[workspace 99 silent] env CODEX_GUI_GUARD=1' "$LOG" \
    "exec did not mark the launch as Codex-guarded"
  assert_grep "CODEX_VISUAL_WORKSPACE='99'" "$LOG" \
    "exec did not pass the hidden workspace to legacy visual wrappers"
  assert_grep "CHROME_DEVTOOLS_AXI_USER_DATA_DIR=" "$LOG" \
    "exec did not force chrome-devtools-axi onto a dedicated profile"
  assert_grep "'printf' 'two words'" "$LOG" \
    "exec did not preserve spaced arguments in one shell command"
  assert_grep "quote'\\''s" "$LOG" "exec did not preserve single quotes"
  assert_grep "\$dollar" "$LOG" "exec did not preserve dollar-sign argument"
  assert_contains "$output" "launched on workspace 99" "exec did not report target workspace"
  pass "fm-visual-guard.sh: exec preserves arguments and uses silent workspace dispatch"
}

test_screenshot_routes_to_headless_output() {
  local shot
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  shot="$TMP_ROOT/shot.png"
  run_guard screenshot "$shot" >/dev/null || fail "screenshot failed"
  assert_present "$shot" "screenshot file was not written by grim"
  assert_grep $'grim\t-o\tCODEX-HEADLESS\t'"$shot" "$LOG" \
    "screenshot did not target CODEX-HEADLESS"
  pass "fm-visual-guard.sh: screenshot targets CODEX-HEADLESS"
}

test_clients_filters_codex_visual_clients() {
  local output
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  : > "$MONITOR_STATE"
  output=$(run_guard clients)
  assert_contains "$output" $'0xabc\tFirstmateVisual\tPreview\t99\tCODEX-HEADLESS' \
    "clients did not include FirstmateVisual client"
  assert_not_contains "$output" "0xdef" "clients included non-Codex visible workspace client"
  pass "fm-visual-guard.sh: clients reports only Codex visual clients"
}

test_browser_uses_independent_firstmate_visual_class() {
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  run_guard browser "http://127.0.0.1:5173" >/dev/null || fail "browser failed"
  assert_grep "'google-chrome-stable' '--class=FirstmateVisual'" "$LOG" \
    "browser did not launch Chrome with FirstmateVisual class on workspace 99"
  assert_grep "--user-data-dir=" "$LOG" "browser did not set an isolated user-data-dir"
  pass "fm-visual-guard.sh: browser uses an independent FirstmateVisual Chrome launch"
}

test_exec_corrals_legacy_codex_visual_leak() {
  local output
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  : > "$LEAK_STATE"
  output=$(run_guard exec -- xdg-open "https://example.test" 2>&1)
  assert_absent "$LEAK_STATE" "legacy CodexVisual leak marker was not cleared"
  assert_grep $'hyprctl\tdispatch\tmovetoworkspacesilent\t99,address:0xced' "$LOG" \
    "guard did not move the legacy CodexVisual client off visible workspace 11"
  assert_no_grep "99,address:0xdef" "$LOG" \
    "guard tried to move the user's ordinary Chrome window"
  assert_contains "$output" "moved Codex visual client 0xced" \
    "guard did not report remediation of the legacy visible client"
  pass "fm-visual-guard.sh: exec corrals legacy codex-visual-browser workspace leaks"
}

test_exec_exits_early_when_visual_clients_already_hidden() {
  local count
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE" "$LEAK_STATE" "$CLIENT_COUNT_STATE"
  FM_VISUAL_VERIFY_ATTEMPTS=5 run_guard exec -- xdg-open "https://example.test" >/dev/null \
    || fail "exec with already-hidden client failed"
  count=$(cat "$CLIENT_COUNT_STATE")
  [ "$count" -lt 5 ] || fail "verification exhausted attempts despite already-hidden client; clients calls=$count"
  assert_no_grep $'hyprctl\tdispatch\tmovetoworkspacesilent' "$LOG" \
    "guard moved a client even though every Codex visual client was already hidden"
  pass "fm-visual-guard.sh: exits verification early after matching clients are already hidden"
}

test_exec_waits_for_late_visual_client_before_exiting() {
  local count output
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE" "$CLIENT_COUNT_STATE"
  : > "$LEAK_STATE"
  output=$(FM_VISUAL_TEST_CLIENT_MODE=late-leak FM_VISUAL_VERIFY_ATTEMPTS=6 run_guard exec -- xdg-open "https://example.test" 2>&1) \
    || fail "exec with late-appearing client failed"
  assert_absent "$LEAK_STATE" "late CodexVisual leak marker was not cleared"
  count=$(cat "$CLIENT_COUNT_STATE")
  [ "$count" -ge 3 ] || fail "verification exited before the late Codex visual client appeared; clients calls=$count"
  [ "$count" -lt 6 ] || fail "verification did not exit after remediating the late Codex visual client; clients calls=$count"
  assert_grep $'hyprctl\tdispatch\tmovetoworkspacesilent\t99,address:0xced' "$LOG" \
    "guard did not move the late legacy CodexVisual client"
  assert_contains "$output" "moved Codex visual client 0xced" \
    "guard did not report remediation of the late visible client"
  pass "fm-visual-guard.sh: keeps settling until a late visual client appears and is hidden"
}

test_refuses_normal_user_workspace_by_default() {
  local output code
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  output=$(FM_VISUAL_TEST_LOG="$LOG" \
    FM_VISUAL_TEST_MONITOR_STATE="$MONITOR_STATE" \
    FM_VISUAL_HYPRCTL=hyprctl \
    FM_VISUAL_GRIM=grim \
    FM_VISUAL_WORKSPACE=11 \
    PATH="$FAKEBIN:$PATH" \
    "$ROOT/bin/fm-visual-guard.sh" ensure 2>&1)
  code=$?
  [ "$code" -ne 0 ] || fail "ensure unexpectedly accepted user workspace 11"
  assert_contains "$output" "refusing to use user workspace '11'" \
    "ensure did not explain the user-workspace refusal"
  pass "fm-visual-guard.sh: refuses normal user workspace by default"
}

test_doctor_fails_when_hyprctl_missing() {
  local output code
  output=$(FM_VISUAL_HYPRCTL="$TMP_ROOT/no-hyprctl" "$ROOT/bin/fm-visual-guard.sh" doctor 2>&1)
  code=$?
  [ "$code" -ne 0 ] || fail "doctor unexpectedly passed without hyprctl"
  assert_contains "$output" "missing required command: hyprctl" \
    "doctor did not explain missing hyprctl"
  pass "fm-visual-guard.sh: doctor fails clearly without hyprctl"
}

test_doctor_checks_screenshot_dependency() {
  local output code
  write_fake_tools
  rm -f "$LOG" "$MONITOR_STATE"
  output=$(FM_VISUAL_TEST_LOG="$LOG" \
    FM_VISUAL_TEST_MONITOR_STATE="$MONITOR_STATE" \
    FM_VISUAL_HYPRCTL=hyprctl \
    FM_VISUAL_GRIM="$TMP_ROOT/no-grim" \
    PATH="$FAKEBIN:$PATH" \
    "$ROOT/bin/fm-visual-guard.sh" doctor 2>&1)
  code=$?
  [ "$code" -ne 0 ] || fail "doctor unexpectedly passed without grim"
  assert_contains "$output" "missing required command: grim" \
    "doctor did not explain missing grim"
  pass "fm-visual-guard.sh: doctor fails clearly without grim"
}

test_exec_preserves_arguments_and_silent_workspace
test_screenshot_routes_to_headless_output
test_clients_filters_codex_visual_clients
test_browser_uses_independent_firstmate_visual_class
test_exec_corrals_legacy_codex_visual_leak
test_exec_exits_early_when_visual_clients_already_hidden
test_exec_waits_for_late_visual_client_before_exiting
test_refuses_normal_user_workspace_by_default
test_doctor_fails_when_hyprctl_missing
test_doctor_checks_screenshot_dependency
