# Milestone 7 — Saved Queries MVP

Saved queries are global to the user, independent of PromQL history, and shared
by all Prometheus and VictoriaMetrics connections.

## Usage

| Context | Key | Action |
| --- | --- | --- |
| Editor | `Ctrl+S` | Save the editor expression under a new name |
| PromQL editor or Results | `Ctrl+O` | Open Saved Queries |
| Browser | `↑` / `↓` | Select a query |
| Browser | `/` | Search by case-insensitive name substring |
| Search | `Enter` / `Esc` | Keep or cancel the filter |
| Browser | `Enter` | Load into the editor without running |
| Browser | `e` | Rename the selected query |
| Browser | `u` | Confirm replacing its expression with the current editor contents |
| Browser | `d` | Confirm deleting the selected query |
| Dialog | `Enter` / `Esc` | Confirm or cancel |
| Browser | `Esc` | Close and restore previous focus |

Save, browse, and confirmation are floating overlays; they don't move or resize
the editor, results, or graph. Loading changes only the editor and its focus. Run
the loaded query explicitly with `Ctrl+Enter` or `Ctrl+G`. Rename, update,
and delete leave the browser open. If a renamed query no longer matches the active
filter, the filter clears so the renamed query remains selected.

## Storage

The file is `~/.local/share/promtui/queries.yaml`, next to the default query
history file. `-config` and `-history` do not move it.

```yaml
queries:
  - name: Request rate
    expression: |-
      sum by (job) (
        rate(http_requests_total[5m])
      )
```

The app creates the file on the first successful save. Names are trimmed,
nonempty, and unique regardless of case. Expressions retain their original
whitespace, and entries retain their order. Writes go to a private temporary
file in the same directory; the app syncs and closes it, compares the current
file with the opened snapshot, then atomically renames the temporary file into
place. An update with identical text does not rewrite the file. If the file
changed, close and reopen Saved Queries before retrying. Invalid YAML is not
overwritten; fix or back up the file before retrying.

## Verification and limits

- `go test ./...` covers storage, exact multiline round trips, no-op updates,
  atomic replacement and preservation on failure, editor focus, search,
  confirmations, and overlay bounds and Kitty placement.
- A Rex terminal smoke test verified `Ctrl+S` and `Ctrl+O` through the terminal
  input path and loading without execution. Visually verify
  Kitty layering on a live range graph: open and close each saved-query overlay,
  then resize and repeat. The uncovered graph should stay in place and no
  stale image pixels should remain.
- External edits are checked just before replacement; this is not a lock
  against another process writing during the final check/rename window.
  Manually added expressions still need to fit the existing editor.
- Parameters, templates, tags, folders, per-connection collections, sync,
  direct execution from the browser, and query versioning are out of scope.
