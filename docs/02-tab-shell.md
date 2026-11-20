# Milestone 4 — Multi-Tab Application Shell

## Objective

Extend PromTUI from a single-purpose PromQL explorer into a multi-view terminal application for Prometheus and VictoriaMetrics.

This milestone implements only the application shell, tab navigation and placeholder views.

Do not implement Alerts or Targets API integration yet.

## 1. Top-Level Tabs

Introduce three tabs:

| ID | Name | Description |
|---|---|---|
| 0 | PromQL | Existing query explorer |
| 1 | Alerts | Placeholder for active alerts |
| 2 | Targets | Placeholder for scrape targets |

The PromQL tab remains the default at startup.

Example header:

```text
 PromTUI  q1q1 │ [0 PromQL]  1 Alerts  2 Targets          Range · Last 1h
```

The active tab must be visually distinguishable using foreground/background styling.

Inactive tabs should remain readable.

Use the existing application color palette.

## 2. Keyboard Navigation

### Tab switching

- `0`, `1`, `2` switch top-level tabs when focus is not in a text input.
- `Alt+0`, `Alt+1`, `Alt+2` switch tabs globally, including while editing PromQL, if Rex supports these combinations.
- Numeric shortcuts must never insert unwanted characters into the PromQL editor or interfere with query entry.
- `Tab` and `Shift+Tab` remain reserved for switching panels within the active tab.
- `Esc` must not switch top-level tabs.

Do not overload `Tab` with both panel navigation and top-level tab navigation.

### Focus

Each tab owns its own focus state.

Switching away from a tab and returning must restore its previous focus.

The focused panel must retain the existing highlighted border behavior.

### Footer

Preserve the context-sensitive footer.

It must display actions relevant to the active tab and focused component.

Do not display every shortcut permanently.

## 3. PromQL Tab

Move the existing PromQL interface into tab 0 without changing its functionality.

Preserve:

- Multiline PromQL editor
- Syntax highlighting and autocomplete
- Query history
- Instant and range queries
- Graph and table views
- Series selection and visibility
- Time-range selection
- Labels inspection
- Existing floating overlays
- Keyboard shortcuts
- Kitty graphics rendering

### State preservation

Switching to another tab and returning must preserve:

- Query text
- Cursor position
- Query results
- Graph/table selection
- Selected series
- Hidden/visible series
- Time range
- Panel focus

Switching tabs must not automatically re-execute the query.

## 4. Alerts Placeholder

Tab 1 displays a simple placeholder inside a bordered panel.

Example:

```text
╭─ Alerts ───────────────────────────────────────╮
│                                                │
│   Active Alerts                                │
│                                                │
│   Alert exploration will be implemented        │
│   in a future milestone.                       │
│                                                │
╰────────────────────────────────────────────────╯
```

Requirements:

- Uses the same panel styling as PromQL.
- Displays the active connection name.
- Supports switching to other tabs.
- Has an appropriate context-sensitive footer.
- Does not make API requests.
- Does not implement fake alert data.

## 5. Targets Placeholder

Tab 2 displays a similar placeholder.

Example:

```text
╭─ Targets ──────────────────────────────────────╮
│                                                │
│   Scrape Targets                               │
│                                                │
│   Target exploration will be implemented       │
│   in a future milestone.                       │
│                                                │
╰────────────────────────────────────────────────╯
```

Requirements are identical to the Alerts placeholder.

Do not implement target discovery yet.

## 6. Application Architecture

Separate application-wide state from tab-specific state.

Suggested conceptual structure:

```text
App
├── Shell
│   ├── Header and tab bar
│   ├── Active tab
│   ├── Global key routing
│   ├── Shared overlay handling
│   └── Context-sensitive footer
│
├── PromQL
│   ├── Existing editor and results
│   └── PromQL-specific state
│
├── Alerts
│   └── Placeholder
│
└── Targets
    └── Placeholder
```

Do not force a large architectural rewrite if the existing Bubble Tea model can be extended cleanly.

The shell should be responsible for:
- Active tab selection
- Global shortcuts
- Shared layout
- Tab lifecycle
- Routing keyboard events to the active tab

Individual tabs should own:
- Their internal state
- Panel focus
- View-specific keyboard handling
- View-specific footer actions

Keep the architecture extensible for future tabs.

## 7. Backend Compatibility

Prometheus and VictoriaMetrics are equal first-class backends.

Do not introduce backend-specific assumptions into the tab shell.

The current connection configuration and API client must continue to work.

Future Alerts and Targets implementations may require backend-specific capabilities.

The architecture should allow capability detection or backend-specific adapters later, but **do not implement those abstractions prematurely**.

A backend that does not expose a particular feature should eventually display an informative unsupported/unavailable state rather than fail the entire application.

## 8. Kitty Graphics Lifecycle

This is a critical regression area.

When switching away from PromQL:

- Hide or remove the active Kitty graph placement.
- Do not leave graph images visible over Alerts or Targets.
- Preserve the underlying query result and graph state.

When switching back:

- Restore the graph correctly.
- Use the current terminal dimensions.
- Do not re-execute PromQL unnecessarily.
- Do not leave stale image placements.

Repeated switching between tabs must not cause flickering, image accumulation or rendering corruption.

## 9. Layout

Use a compact header containing:

- Application name
- Active connection
- Available tabs
- Relevant global status

Avoid adding multiple redundant header rows.

The active tab should be obvious.

Preserve the existing bordered-panel style.

The footer must remain anchored to the bottom of the terminal.

The application must handle terminal resizing correctly.

## 10. Testing

Add tests for:

- Tab selection and key routing
- Numeric keys inside the PromQL editor
- State preservation across tab switches
- Focus restoration
- Footer context changes
- Terminal resizing
- No unnecessary query execution when switching tabs

Manual Rex verification:

1. Start in PromQL.
2. Enter and execute a range query.
3. Switch to Alerts.
4. Switch to Targets.
5. Return to PromQL.
6. Verify the original graph and query are preserved.
7. Repeat switching several times.
8. Verify no Kitty image artifacts.
9. Verify numeric keys can be typed normally in the editor.
10. Verify Tab still switches between Query and Results panels.

## 11. Acceptance Criteria

Milestone 4 is complete when:

- Three top-level tabs are available.
- PromQL remains fully functional.
- Alerts and Targets display placeholders.
- Tab switching is predictable.
- Active tab and focused panel are visually obvious.
- Existing PromQL state survives tab changes.
- Context-sensitive footer remains functional.
- Kitty graphics render correctly after switching tabs.
- Both Prometheus and VictoriaMetrics configurations remain supported.
- No Alerts or Targets API integration has been introduced.

## 12. Explicit Non-Goals

Do not implement:

- Alerts API
- Targets API
- Alertmanager integration
- Auto-refresh
- Alert filtering
- Target filtering
- Additional dashboards
- New graph functionality
- Backend capability detection

The purpose of this milestone is to establish a stable, extensible application shell.

## 13. Documentation

After implementation, create `docs/MILESTONE-4.md` documenting:

- Architecture changes
- Tab lifecycle
- Keyboard routing
- State preservation
- Kitty graphics handling
- Tests and manual verification
- Known limitations
- Recommendations for the next milestone

Do not automatically proceed to Alerts or Targets implementation.
