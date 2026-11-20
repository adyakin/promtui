# Milestone 7 — Saved Queries MVP

**Project:** PromTUI
**Scope:** Global saved queries, local persistence, basic CRUD and PromQL editor integration

## 1. Objective

Implement a minimal Saved Queries feature allowing users to save frequently used PromQL expressions under meaningful names and reuse them later.

Supported operations:

- Save a new query
- Browse saved queries
- Load a saved query into the editor
- Rename a saved query
- Update an existing saved query
- Delete a saved query

Saved queries are global and available across all configured Prometheus and VictoriaMetrics connections.

Keep this milestone focused. Do not implement query parameters, templates or advanced query management.

Before implementation, inspect the existing PromQL editor, query history, keyboard handling, configuration and all applicable `docs/uix*.md` files.

Follow the established UI conventions.

## 2. Storage

Store saved queries in a separate YAML file:

```text
~/.local/share/promtui/
├── history.jsonl
└── queries.yaml
```

Use the application's default history directory, regardless of any `-config`
or `-history` override.

Example:

```yaml
queries:
  - name: CPU load server deb
    expression: |
      100 * (1 - avg by (instance) (
        rate(node_cpu_seconds_total{mode="idle",instance="deb:9100"}[5m])
      ))

  - name: Memory available
    expression: |
      node_memory_MemAvailable_bytes
```

### Requirements

- Saved queries are global, not associated with individual connections.
- Store only the query name and PromQL expression.
- Preserve expressions exactly, including multiline formatting.
- Preserve existing query ordering.
- Create the file automatically when saving the first query.
- Missing file means an empty collection, not an error.
- Malformed YAML must produce a clear error.
- Do not silently overwrite or discard corrupted data.
- Write changes atomically using a temporary file and rename.
- Use appropriate file permissions.
- Avoid unnecessary dependencies.

Do not store query results, connection URLs, graph state, query history or credentials.

## 3. Saved Query Identity

Each saved query must have a unique, nonempty name.

Names are case-insensitively unique.

For example, `CPU Load` and `cpu load` must not be treated as two separate saved queries.

Trim surrounding whitespace when validating names.

Do not use the query expression as the identity.

Renaming a query must preserve its expression.

## 4. Save Query

Provide a shortcut from the PromQL editor to save the current expression.

Suggested shortcut: `Ctrl+S`.

Open a true floating overlay:

```text
╭─ Save Query ──────────────────────────╮
│                                      │
│ Name                                 │
│ [CPU load server deb______________]  │
│                                      │
│ Enter Save       Esc Cancel          │
╰──────────────────────────────────────╯
```

Requirements:

- Save the current editor expression.
- Reject empty or whitespace-only expressions.
- Require a nonempty name.
- Reject duplicate names.
- Display validation errors inside the overlay.
- Preserve the editor contents and cursor position.
- Do not execute the query when saving.
- Show a brief confirmation after successful save.

The overlay must not resize or replace the underlying PromQL interface.

## 5. Saved Queries Browser

Provide a shortcut to open the saved queries browser.

Suggested shortcut: `Ctrl+O`.

Use a floating overlay, not a new top-level tab.

Example:

```text
╭─ Saved Queries ───────────────────────────────╮
│                                              │
│ Search: cpu                                  │
│                                              │
│ > CPU load server deb                        │
│   CPU usage all servers                      │
│   CPU throttling                             │
│                                              │
│ ↑↓ Select   Enter Load   e Rename           │
│ u Update    d Delete    Esc Close            │
╰──────────────────────────────────────────────╯
```

### Navigation

- `↑ / ↓` selects a saved query.
- `Enter` loads the selected expression.
- `/` activates search.
- `e` renames the selected query.
- `u` updates the selected query from the current editor.
- `d` deletes the selected query after confirmation.
- `Esc` closes the browser or cancels the active dialog.

Follow existing keyboard conventions where necessary.

### Search

Implement case-insensitive substring matching against query names.

No fuzzy-search dependency is required.

Filtering must not modify stored queries.

Preserve the selected item where possible.

## 6. Load Query

When loading a saved query:

1. Replace the current PromQL editor contents with the saved expression.
2. Close the browser.
3. Restore focus to the editor.
4. Position the cursor consistently with existing editor behavior.
5. Do not execute the query automatically.

Loading must not change:

- Current connection
- Time range
- Graph/table mode
- Query history
- Existing query results

The user explicitly executes the query using the existing execution shortcut.

Do not add a separate execute-from-browser action in this milestone.

## 7. Rename Query

Press `e` on a selected saved query.

Open a compact floating dialog with the current name prefilled.

Requirements:

- Allow changing only the name.
- Preserve the expression.
- Reject empty or duplicate names.
- Save changes immediately after confirmation.
- Preserve the browser selection.
- `Esc` cancels without changes.

## 8. Update Query

Press `u` on a selected saved query to replace its stored expression with the current PromQL editor contents.

This operation must be explicit.

Show a confirmation dialog:

```text
╭─ Update Saved Query ─────────────────╮
│                                     │
│ Replace the stored expression for   │
│ "CPU load server deb" with the       │
│ current editor contents?            │
│                                     │
│ Enter Update      Esc Cancel        │
╰─────────────────────────────────────╯
```

Requirements:

- Use the current editor expression.
- Reject empty expressions.
- Preserve the saved query name.
- Do not execute the query.
- Do not modify the editor.
- Do not update another query accidentally.
- Keep the browser open after updating.
- Display a brief success indication.

If the editor expression is identical to the stored expression, avoid unnecessary file writes.

Do not implement automatic dirty tracking or automatic updates.

## 9. Delete Query

Press `d` on a selected saved query.

Require confirmation before deletion.

Example:

```text
╭─ Delete Saved Query ────────────────╮
│                                    │
│ Delete "CPU load server deb"?       │
│                                    │
│ Enter Delete      Esc Cancel       │
╰────────────────────────────────────╯
```

Requirements:

- Delete only the selected query.
- Do not modify the current editor.
- Preserve other saved queries.
- Keep the browser open after deletion.
- Select the nearest remaining query.
- Show an empty state if the last query is deleted.
- `Esc` cancels without changes.

## 10. Keyboard and Focus Handling

Suggested shortcuts:

| Context | Key | Action |
|---|---|---|
| PromQL | `Ctrl+S` | Save current query |
| PromQL | `Ctrl+O` | Open saved queries |
| Browser | `↑ / ↓` | Navigate |
| Browser | `Enter` | Load |
| Browser | `/` | Search |
| Browser | `e` | Rename |
| Browser | `u` | Update |
| Browser | `d` | Delete |
| Dialog | `Enter` | Confirm |
| Dialog | `Esc` | Cancel |
| Browser | `Esc` | Close |

Verify actual key support in Rex and existing Bubble Tea input handling.

Do not break:

- Multiline editor behavior
- Query execution shortcuts
- Autocomplete
- Query history
- Top-level tab navigation
- Existing floating overlays

Preserve the context-sensitive footer and make it reflect the active browser or dialog.

## 11. UI Requirements

Follow all applicable `docs/uix*.md` specifications.

All dialogs must be true floating overlays.

Requirements:

- No resizing or reflowing the underlying graph.
- Consistent borders and active-state styling.
- Clearly highlighted selected row.
- Compact dialogs for Save, Rename, Update and Delete.
- Scrollable browser for large collections.
- Graceful handling of narrow terminals.
- Correct Kitty graphics layering and cleanup.
- Restore focus correctly when closing overlays.

Do not introduce a new UI framework.

## 12. Error Handling

Handle:

- Missing saved queries file
- Invalid YAML
- Permission errors
- Failed writes
- Duplicate names
- Empty names
- Empty expressions
- Unexpected file changes

Do not silently discard existing queries on read or write failures.

A failed save, update, rename or delete must leave the existing stored collection intact.

Show actionable errors without crashing the application.

## 13. Testing

### Storage tests

- Save and load queries
- Multiline expression preservation
- Rename
- Update
- Delete
- Duplicate name rejection
- Case-insensitive uniqueness
- Missing file
- Malformed YAML
- Atomic writes
- Write failures

### UI tests

- Save dialog
- Browser navigation
- Search
- Load into editor
- Rename dialog
- Update confirmation
- Delete confirmation
- Cancellation
- Empty states
- Focus restoration
- Context-sensitive footer

### Regression tests

- PromQL execution remains unchanged.
- Query history remains independent.
- Switching tabs preserves editor state.
- Existing overlays still work.
- Kitty graphics are not corrupted.

## 14. Non-Goals

Do not implement:

- Query parameters or variables
- Query templates
- Connection-specific saved queries
- Query folders or categories
- Tags
- Descriptions
- Favorites
- Import/export commands
- Cloud synchronization
- Saved query execution directly from the browser
- Automatic query updates
- Query version history
- New top-level tabs
- Rex-specific integrations

These can be considered separately.

## 15. Documentation

Create `docs/MILESTONE-7.md` documenting:

- Storage format and location
- Supported operations
- Keyboard shortcuts
- UI behavior
- Tests performed
- Known limitations
- Deferred functionality

Update user-facing documentation with Saved Queries usage.

Do not automatically proceed to another milestone.

## 16. Acceptance Criteria

Milestone 7 is complete when:

- Users can save a PromQL expression with a unique name.
- Saved queries persist across application restarts.
- Saved queries are available across all connections.
- Users can browse and search saved queries.
- Loading restores the expression without executing it.
- Users can rename, update and delete saved queries.
- Destructive operations require confirmation.
- Storage errors do not destroy existing data.
- Existing PromQL functionality remains intact.
- UI behavior follows established overlay conventions.
- Documentation and tests are complete.
