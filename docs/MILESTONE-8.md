# Milestone 8 — Terminal Program Status (OSC 7501)

PromTUI reports explicit PromQL query activity to the terminal. It does not
report Alerts or Targets refreshes, autocomplete, saved-query operations, or
routine UI updates. Reporting does not change keyboard shortcuts, focus,
results, or the detailed error messages shown in PromTUI.

## Protocol and lifecycle

The encoder follows the [OSC 7501 Program Status Protocol, revision 0.4](https://www.superlogical.com/rex/docs/build/program-status).
Reports use `ESC ] 7501 ; state=…:id=promtui:app=promtui[:msg=…] ESC \`.
Messages are standard base64-encoded UTF-8; controls and oversized messages
are rejected. The terminal record ID is fixed and is separate from the internal
query execution ID.

| Event | State | Message |
| --- | --- | --- |
| Query starts or replaces an older query | `working` | Executing PromQL query |
| Query succeeds, even with zero series | `done` | Query completed |
| Query fails or times out | `error` | Query failed |
| Ctrl+C or a connection change cancels a query | `idle` | None |
| PromTUI exits after sending a report | `clear` | None |

The existing query execution ID invalidates late completions when a query is
cancelled or replaced. Reports never include PromQL text, credentials, or
backend error details. The exit report uses `state=clear:id=promtui`, which
leaves the terminal's root record and unrelated records untouched.

## Configuration and output

Enabled by default when stdout is a TTY. To disable it in YAML:

```yaml
terminal:
  program_status: false
```

For TOML, set `program_status = false` in `[terminal]`.

Status reports use Bubble Tea's `tea.Raw` output path, which also handles Kitty
graphics. Only one status packet is in flight at a time; any transitions that
arrive while it is pending are sent in order on acknowledgement. Reports are
not part of the rendered view. Exit cleanup runs after Bubble Tea stops
rendering, through the same post-run path as Kitty cleanup.

No terminal-specific integration or feature probe is used. Terminals that
don't support OSC 7501 should ignore it. A remote SSH session needs a PTY
(e.g. `ssh -t`) and a terminal path that forwards OSC 7501 unchanged.
Multiplexers or other intermediaries may filter it. An uncatchable exit such
as `SIGKILL` cannot send a clear report.

## Verification

- `go test -race ./...` passed. Tests cover exact encoding and reset, config
  defaults and opt-out, empty results, failure, cancellation, stale
  completions, and exclusion of background activity. A real Bubble Tea event
  loop test checks report order, intact Kitty upload bytes, editor input,
  stale result removal, detailed TUI errors, and exit cleanup.
- **Rex, local:** Verified the terminal's decoded `working`, `done`, `idle`,
  and `error` records against a local test backend, and verified that
  Ctrl+Q removed the record. The captured TUI showed the edited query,
  range graph legend, and detailed backend error. Pixel-level graph and
  flicker checks still need a visual pass.
- **Local PTY:** With terminal-specific environment variables removed, an
  empty query reported `done` and SIGTERM emitted the scoped `clear`.
- **iTerm2, unsupported terminal, and remote SSH:** Not manually verified.

For a visual check, run a range query in a supported terminal, look for
`working` and then `done` in the terminal's status UI, and confirm that the
chart and editor stay intact. Repeat with a failed query and a cancelled
slow query. Quit and confirm the PromTUI status is gone. Repeat over SSH
with a PTY to check the actual terminal path.
