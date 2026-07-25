# Firstmate TUI

`fm-tui` is an opt-in terminal view and selected-worker message surface over one Firstmate operational home.
The first slice lists active workers, resolves managed current state through `bin/fm-crew-state.sh`, shows durable reports, and shows bounded status-event and worker-capture output.
It can send one bounded message to the selected active worker.
It does not launch tasks, attach panes, edit policy, merge work, or change Firstmate lifecycle state.

## Run

Run the TUI from the Firstmate code root with Go 1.25 or newer:

```sh
go run ./cmd/fm-tui --home /absolute/path/to/firstmate-home
```

`--home` takes precedence over `FM_HOME`.
When both are absent, the code root is used as the operational home.
Use `--root` when the Firstmate scripts come from a different code root.

In a managed worktree where Go cannot derive repository metadata from Git's shared directory, add `-buildvcs=false` immediately after `go run`.

## Snapshot

The snapshot mode prints stable, ANSI-free output for tests and review:

```sh
go run ./cmd/fm-tui --snapshot --home /absolute/path/to/firstmate-home
```

The output orders Firstmate-managed workers by task id, then captain-private Direct Codex sessions by target, and selects the first worker for the inspector.
It includes ownership, send route, the durable report view, and bounded live sources without sending a message.

## Ownership and active filtering

The selector and selected-worker header always identify one of two ownership groups.

- `Firstmate managed` means the worker is backed by this home's metadata, current-state adapter, and recovery-grade backend agent-state probe.
- `Captain private / Direct Codex` means a live tmux Codex pane is not claimed by any Firstmate metadata window.

Managed records are visible only while their backend agent state is `alive` and their authoritative current state is nonterminal.
Done, failed, cancelled, missing, dead, and stale metadata or status-only records are excluded.
The last status event is never promoted to current-state truth.

Direct discovery uses a bounded tmux pane inventory and the established `pane_current_command` Codex signal.
It excludes every tmux window claimed by `state/*.meta` before rendering or sending.
Direct discovery is tmux-only in this slice and does not attempt to discover Codex Desktop threads or private sessions on other backends.

## Keys

| Key | Action |
| --- | --- |
| `j`, `down` | Select the next task |
| `k`, `up` | Select the previous task |
| `g`, `G` | Select the first or last task |
| `left`, `right` | Show Reports or Live |
| `enter` | Switch between Reports and Live, or send while the composer is focused |
| `esc` | Close help, blur the composer without clearing its draft, or return to Reports |
| `i` | Focus the selected-worker message composer |
| `r` | Refresh metadata, current state, reports, events, and the selected live capture |
| `?` | Toggle keyboard help |
| `q` | Quit |

Reports reads only `data/<id>/report.md` and states explicitly when no durable report is present.
Live labels `state/<id>.status` as bounded historical events and never treats its last line as current-state truth.
The worker capture is bounded by `--live-lines` and comes through `bin/fm-peek.sh`.
Private Direct Codex capture comes through `bin/fm-tui-direct.sh`.

## Message composer

The compact composer remains visible below the dominant Reports or Live output.
Press `i`, type a message, and press `enter` to send only to the selected active worker.
The message is limited to 4096 bytes.
Empty, oversized, invalid-target, and mismatched-ownership submissions are rejected before adapter execution.
Adapter commands receive the target and message as separate arguments and run with timeouts and bounded output.

Firstmate-managed messages route through `bin/fm-send.sh`, preserving its target resolution, submission checks, secondmate markers, pending-reply records, and audit behavior.
Captain-private messages route separately through `bin/fm-tui-direct.sh`, which revalidates that the exact tmux pane is live, runs Codex, and is not claimed by Firstmate metadata.
A successful send clears the submitted draft.
A failed send leaves the draft intact and displays the adapter error in the TUI.

## Bounds and errors

Metadata is read from sorted `state/*.meta` files.
Status history defaults to 20 events and a 64 KiB byte bound per task.
Reports default to a 64 KiB byte bound.
Read and send adapter output is capped.
Read adapter failures are shown as unavailable or unknown rather than replaced with a status-log guess.
