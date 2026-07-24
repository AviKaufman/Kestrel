# Firstmate TUI

`fm-tui` is an opt-in, read-only terminal view over one Firstmate operational home.
The first slice lists task metadata, resolves current state through `bin/fm-crew-state.sh`, shows durable reports, and shows bounded status-event and worker-capture output.
It does not launch tasks, steer workers, attach panes, edit policy, merge work, or change Firstmate lifecycle state.

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

The output orders tasks by id and selects the first task for the inspector.
It includes both the durable report view and the bounded live sources so the read-only contract can be reviewed without an interactive terminal.

## Keys

| Key | Action |
| --- | --- |
| `j`, `down` | Select the next task |
| `k`, `up` | Select the previous task |
| `g`, `G` | Select the first or last task |
| `left`, `right` | Show Reports or Live |
| `enter` | Switch between Reports and Live |
| `esc` | Close help or return to Reports |
| `r` | Refresh metadata, current state, reports, events, and the selected live capture |
| `?` | Toggle keyboard help |
| `q` | Quit |

Reports reads only `data/<id>/report.md` and states explicitly when no durable report is present.
Live labels `state/<id>.status` as bounded historical events and never treats its last line as current-state truth.
The worker capture is bounded by `--live-lines` and comes through `bin/fm-peek.sh`.

## Bounds and errors

Metadata is read from sorted `state/*.meta` files.
Status history defaults to 20 events and a 64 KiB byte bound per task.
Reports default to a 64 KiB byte bound.
Adapter output is capped, and adapter failures are shown as unavailable or unknown rather than replaced with a status-log guess.
