# Firstmate TUI

`fm-tui` is an opt-in terminal hub, worker view, and bounded message surface over one Firstmate operational home.
The first slice keeps the Firstmate hub available with zero workers, separates active managed workers from captain-private Codex threads, resolves managed current state through `bin/fm-crew-state.sh`, shows durable reports, and shows bounded hub, status-event, and worker-capture output.
It can send one bounded message to the hub or selected active worker and can create a captain-private Codex thread.
It does not launch Firstmate-managed tasks, steer lifecycle state, attach panes, edit policy, merge work, or write task metadata.

## Run

Run the TUI from the Firstmate code root with Go 1.25 or newer:

```sh
go run ./cmd/fm-tui --home /absolute/path/to/firstmate-home
```

`--home` takes precedence over `FM_HOME`.
When both are absent, the code root is used as the operational home.
Use `--root` when the Firstmate scripts come from a different code root.
Use `--private-cwd /absolute/project/path` to choose the only working directory accepted by the private-thread creation action.
When `--private-cwd` is absent, the code root is used.

In a managed worktree where Go cannot derive repository metadata from Git's shared directory, add `-buildvcs=false` immediately after `go run`.

## Snapshot

The snapshot mode prints stable, ANSI-free output for tests and review:

```sh
go run ./cmd/fm-tui --snapshot --home /absolute/path/to/firstmate-home
```

The output shows the compact header, active Firstmate hub destination, hub authority and bounded conversation history, separate managed and private counts, applicable send routes, the non-executing `n` action, and the worker inspector when a worker is present.
It orders Firstmate-managed workers by task id, then captain-private Direct Codex sessions by target.
Snapshot mode never sends a message or launches a thread.

## Destinations, ownership, and active filtering

The compact header exposes three destinations.

- `Firstmate hub` is persistent and routes to the current primary supervisor target resolved by `bin/fm-supervisor-target-lib.sh` and revalidated through `bin/fm-tui-hub.sh`.
- `Managed workers` contains only workers backed by this home's metadata, current-state adapter, and recovery-grade backend agent-state probe.
- `Private Codex` contains only live tmux Codex panes not claimed by any Firstmate metadata window.

Managed records are visible only while their backend agent state is `alive` and their authoritative current state is nonterminal.
Done, failed, cancelled, missing, dead, and stale metadata or status-only records are excluded.
The last status event is never promoted to current-state truth.

Direct discovery uses a bounded tmux pane inventory and the established `pane_current_command` Codex signal.
It excludes every tmux window claimed by `state/*.meta` before rendering or sending.
Direct discovery is tmux-only in this slice and does not attempt to discover Codex Desktop threads or private sessions on other backends.

Hub resolution first honors explicit inherited supervisor authority.
When an active primary TUI process lacks those markers, it can recover the same authority from the live session-lock owner's whitelisted environment without scanning tmux inventory.
That recovery requires a readable Linux `/proc/<pid>/environ`; inherited authority remains the portable path on other systems.
Hub resolution, history, and messaging support tmux and Herdr only in this slice; a primary running directly on cmux or another backend is reported as unavailable.

## Keys

| Key | Action |
| --- | --- |
| `tab`, `shift-tab` | Select the next or previous top-level destination |
| `1`, `2`, `3` | Select Firstmate hub, Managed workers, or Private Codex |
| `j`, `down` | Select the next worker within the active worker destination |
| `k`, `up` | Select the previous worker within the active worker destination |
| `g`, `G` | Select the first or last worker within the active worker destination |
| `left`, `right` | Show Reports or Live for a worker destination |
| `enter` | Switch between worker Reports and Live, or send while the composer is focused |
| `esc` | Close help, cancel private creation, blur the composer without clearing its draft, or return to Reports |
| `i` | Focus the active hub or selected-worker message composer |
| `n` | Open the contained new-private-Codex label prompt |
| `r` | Refresh metadata, hub history, current state, reports, events, and the selected capture when Live is visible |
| `?` | Toggle keyboard help |
| `q`, `ctrl-c` | Quit |

Reports reads only `data/<id>/report.md` and states explicitly when no durable report is present.
Live labels `state/<id>.status` as bounded historical events and never treats its last line as current-state truth.
The worker capture is bounded by `--live-lines` and comes through `bin/fm-peek.sh`.
Private Direct Codex capture comes through `bin/fm-tui-direct.sh`.

## Message composer

The compact composer remains visible below the dominant Reports or Live output for worker destinations and in the persistent hub view.
The hub view keeps compact destination and target identity at the top, gives the bounded read-only conversation capture the dominant region, and anchors its composer at the bottom.
Press `i`, type a message, and press `enter` to send only to the active hub or selected worker.
The message is limited to 4096 bytes.
Empty, oversized, invalid-target, and mismatched-ownership submissions are rejected before adapter execution.
Adapter commands receive the target and message as separate arguments and run with timeouts and bounded output.

Firstmate-managed messages route through `bin/fm-send.sh`, preserving its target resolution, submission checks, secondmate markers, pending-reply records, and audit behavior.
Captain-private messages route separately through `bin/fm-tui-direct.sh`, which revalidates that the exact tmux pane is live, runs Codex, and is not claimed by Firstmate metadata.
Hub messages route through `bin/fm-tui-hub.sh`, which resolves the established primary supervisor authority and requires the same live backend target immediately before submission.
Hub history uses the same resolve-and-revalidate path before a bounded backend capture, so stale authority is never read as the current conversation.
A successful send clears the submitted draft.
A failed send leaves the draft intact and displays the adapter error in the TUI.

## Private Codex creation

Press `n` from any destination to open a contained label prompt.
Labels must match `[a-z0-9][a-z0-9-]{0,31}`.
The working directory is the validated absolute `--private-cwd` value and is never accepted from the prompt.
The adapter reuses the current tmux session when the TUI runs inside tmux and otherwise uses the dedicated `firstmate-private` session.
It creates a pinned `codex-<label>` window, launches the installed `codex` command with fixed tmux arguments, writes no `state/*.meta`, and revalidates the concrete Codex pane before returning it.
The TUI then rediscovers the pane and selects it under Private Codex.
Launch or rediscovery failure leaves the prior destination intact, preserves the label for correction, and never fabricates a worker selection.
Private creation is tmux-only in this slice.

## Bounds and errors

Metadata is read from sorted `state/*.meta` files.
Status history defaults to 20 events and a 64 KiB byte bound per task.
Hub conversation history defaults to the `--live-lines` bound and retains the newest available lines.
Reports default to a 64 KiB byte bound.
Read, send, and create adapter output is capped.
Externally sourced metadata, reports, events, captures, hub history, and errors have ANSI and terminal control sequences removed before interactive or snapshot rendering.
Current-state adapter failures are shown as `unknown` when the recovery-grade agent probe still proves the worker alive, while hub and capture failures are shown as unavailable or read errors.
Metadata, event, report, and direct-discovery failures fail the load rather than falling back to a status-log guess.
