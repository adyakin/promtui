# PromTUI — Query Error Handling and Result State Correctness

## Objective

Improve PromTUI's handling of failed and empty PromQL queries.

Implement two related changes:

1. Provide a floating overlay for viewing the complete query error.
2. Ensure Graph and Table never display stale results from a previous query.

Follow all existing `docs/uix*.md` specifications and reuse the application's established overlay conventions.

## 1. Query Result Lifecycle

The Results panel must always represent the latest executed query.

| State | Expected behavior |
|---|---|
| Running | Clear previous results and display `Executing query...` |
| Success with data | Display new results |
| Success without data | Display `No data` |
| Failure / timeout | Clear results and display error summary |
| Cancelled | Keep results cleared; display a neutral cancellation state |

### When a query starts

- Immediately invalidate the previous query's results.
- Clear both Graph and Table data.
- Remove previous Kitty graphics placements.
- Clear previous series selections and labels associated with the results.
- Clear the previous query error.
- Show `Executing query...`.

Do not clear results merely because the user edits the PromQL expression.

### When a query succeeds

- Replace the current result state with the new response.
- Render the corresponding Graph or Table.
- For an empty result set, display `No data`.
- Clear any previous error state.

An empty Prometheus response is not an error.

### When a query fails

- Keep Graph and Table empty.
- Remove any previous Kitty graph placements.
- Store the complete error message.
- Display a concise error summary in the Results panel.
- Make the full error accessible through the Error Details overlay.
- Report failure through OSC 7501.

## 2. Inline Error Summary

The existing single-line error message gets truncated on narrow terminals.

Replace it with a short, readable summary and an explicit shortcut to inspect the complete error.

Example:

```text
Query failed after 30.001s: context deadline exceeded
[e] Error details
```

Requirements:

- Prefer the underlying error cause over long request URLs when possible.
- Preserve the complete original error separately.
- Do not display the entire HTTP request URL in the inline summary.
- Show the query duration when available.
- Ensure the summary fits reasonably within the available terminal width.
- Indicate how to open the full error.

## 3. Error Details Overlay

Introduce a new floating overlay accessible using `e` when a query error exists.

Example:

```text
╭─ Query Error ───────────────────────────────────────╮
│                                                    │
│ Query failed after 30.001s                         │
│                                                    │
│ Full error:                                        │
│                                                    │
│ Post "https://prometheus.example.com/api/v1/       │
│ query_range": context deadline exceeded            │
│ (Client.Timeout exceeded while awaiting headers)   │
│                                                    │
│ ↑↓ Scroll      y Copy      Esc Close               │
╰────────────────────────────────────────────────────╯
```

### Requirements

- Display the complete, original error without truncation.
- Wrap long lines according to the overlay width.
- Support vertical scrolling for lengthy errors.
- Support narrow terminal dimensions gracefully.
- Show query duration when available.
- Preserve multiline error messages.
- Do not replace the underlying Results view or resize the application layout.
- Restore the previous focus when closing.
- Follow existing floating overlay border, selection and styling conventions.
- Ensure the overlay renders above Kitty graphics correctly.

### Keyboard shortcuts

| Key | Action |
|---|---|
| `e` | Open Error Details when an error exists |
| `↑ / ↓` | Scroll error text |
| `PgUp / PgDn` | Scroll by page |
| `y` | Copy complete error |
| `Esc` | Close overlay |

The `e` shortcut should work from Graph and Table views, provided it does not conflict with existing keybindings.

When no error exists, do not show an inactive or misleading shortcut.

### Copying errors

Use an existing clipboard mechanism if one is available.

If no clipboard support exists, use a terminal-compatible implementation, such as OSC 52, with appropriate handling of unsupported terminals.

Copy the complete error text, not the shortened summary.

Do not introduce a Rex-specific dependency.

## 4. Error State Lifetime

- Keep the current error accessible until another query execution starts.
- Opening and closing the overlay must not modify the error.
- Switching between Graph and Table must preserve error state.
- Switching application tabs must not lose the error.
- Starting a new query clears the previous error.
- A successful query clears the error.
- A stale query response must not restore an older error.

## 5. Concurrent Query Execution

Use the existing query execution identity or generation mechanism.

Requirements:

- Only the latest relevant query can update results.
- Only the latest relevant query can update the displayed error.
- Old query completions must not restore cleared graph data.
- Old query failures must not replace a newer successful result.
- OSC 7501 transitions must correspond to the current query.

## 6. Kitty Graphics Cleanup

Clearing Go application state alone is insufficient.

Ensure previous Kitty graphics placements are explicitly removed when:

- A new query starts.
- A query returns no data.
- A query fails.
- A query is cancelled.

Do not accidentally delete unrelated terminal graphics.

Switching between Graph and Table must not reveal an old image.

## 7. Tests

### Error display

- HTTP timeout produces a concise summary.
- Full error is accessible through the overlay.
- Long errors wrap correctly.
- Multiline errors are preserved.
- Scrolling works.
- Copy returns the complete error.
- `Esc` restores the previous focus.
- No error means no Error Details action.

### Result correctness

- Query A succeeds, Query B fails: A's results disappear.
- Query A succeeds, Query B returns empty: show `No data`.
- Query A succeeds, Query B starts: immediately clear A's results.
- Query A fails, Query B succeeds: clear A's error.
- Query A is slow, Query B finishes first: A cannot overwrite B.
- Switching Graph/Table after failure never reveals stale data.
- Kitty graphics placements are removed correctly.

### Regression

- PromQL editor and autocomplete continue working.
- Query history is unchanged.
- Saved Queries remain functional.
- Time range and display mode are preserved.
- Existing floating overlays work correctly.
- OSC 7501 status reporting remains correct.

## 8. Documentation

Document the new error inspection shortcut and query result lifecycle.

Review all applicable `docs/uix*.md` files before implementation.

## Acceptance Criteria

The task is complete when:

- Failed queries never leave previous graph/table results visible.
- Empty successful queries display `No data`.
- Running queries immediately invalidate previous results.
- Errors show a concise inline summary.
- Full errors are accessible through a scrollable floating overlay.
- Users can copy the complete error.
- Error state is preserved until the next query.
- Stale responses cannot overwrite current results or errors.
- Kitty graphics cleanup works correctly.
- Existing functionality remains unchanged.
