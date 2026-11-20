## Floating Overlay Requirements

**Status:** Mandatory

**Applies to:** Time Range, Series Selector, Labels Inspector and future dialogs.

### 1. True Overlay, Not Layout Reflow

All dialogs must be rendered as floating overlays above the existing application.

Opening an overlay must NOT:

- Resize or shrink the graph.
- Change the dimensions of Query or Results panels.
- Reposition the underlying application components.
- Replace the main screen with a separate view.
- Trigger a new PromQL request.
- Modify the current graph viewport or selected series.

The underlying application must retain exactly the same layout and dimensions while an overlay is open.

**An overlay must occupy the same screen coordinates as part of the underlying UI, not additional layout space.**

### 2. Visual Appearance

Every overlay must have:

- A complete rectangular border.
- A title integrated into the top border.
- An accent-colored border indicating active focus.
- A solid background that hides the underlying content.
- A clearly highlighted selected row.
- Padding of approximately one character horizontally.
- A compact size determined by its contents.

The overlay should visually resemble a floating dialog in LazyGit.

Do not use a separate full-screen page or a side-by-side layout.

### 3. Overlay Positioning

**Time Range**

- Centered horizontally and vertically within the Results panel.
- Compact width, approximately 30–40 terminal columns.
- Height determined by the number of presets.
- Does not resize the graph.

**Series Selector**

- Floating over the right side of the Results panel.
- Approximately 35–45% of the Results panel width.
- Vertically centered or aligned near the top.
- Height based on the number of series, capped at approximately 70% of available Results height.
- Scrollable when the list exceeds the available height.
- Graph remains visible in the uncovered area.

**Labels Inspector**

- Compact floating dialog over Results.
- Sized according to the label names and values.
- Long values may be truncated visually, but full values must remain available for copying.
- Scrollable when necessary.
- Does not change graph or table dimensions.

### 4. Rendering and Layering

Overlay rendering must follow this conceptual order:

1. Render the normal Query and Results layout at its original dimensions.
2. Render or preserve the graph at its original dimensions.
3. Composite the overlay at fixed coordinates above the underlying content.
4. Render the context-sensitive footer.

The overlay must visually cover the content beneath it.

Because graphs use the Kitty graphics protocol, ordinary terminal text rendering may not automatically appear above image placements.

The implementation must explicitly handle this.

Acceptable approaches include:

- Managing Kitty image placement and clipping.
- Clearing or temporarily hiding only the image region covered by the overlay.
- Compositing the dialog into the final rendered image when appropriate.
- Using a dedicated overlay compositor with coordinated text and image rendering.

Do not solve the problem by shrinking or relocating the graph.

### 5. Opening and Closing

Opening an overlay:

- Preserves the underlying application state.
- Captures keyboard focus.
- Does not alter the graph's dimensions.
- Does not trigger unnecessary queries.

Closing an overlay:

- Restores focus to the previous panel.
- Restores any covered graph content.
- Leaves no stale Kitty image artifacts.
- Does not cause layout jumps or flickering.

### 6. Series Selector Interaction

The Series Selector must remain open while visibility changes are made.

- `↑ / ↓` selects a series.
- `Space` toggles visibility.
- `Enter` isolates or restores a series.
- `a` shows all series.
- `/` filters the list.
- `Esc` closes the overlay.

Each series must have a colored indicator matching its graph line.

Visibility changes should update the underlying graph immediately, without changing its dimensions or issuing a new query.

### 7. Labels Inspector Interaction

The Labels Inspector should display all labels of the currently selected series.

Example:

```text
╭─ Series labels ─────────────────────╮
│ instance   192.168.1.1             │
│ job        node-exporter           │
│ cluster    production              │
│ environment production             │
│                                    │
│ y Copy labels          Esc Close   │
╰────────────────────────────────────╯
```

The labels must be selectable or copyable without changing the current query or result.

### 8. Context-Sensitive Footer

When an overlay is active, the footer must display only actions relevant to that overlay.

The footer remains at its original screen position.

Opening a dialog must not push the footer downward or change its width.

### 9. Acceptance Criteria

For each of Time Range, Series Selector and Labels Inspector:

1. Capture a screenshot of the normal Results view.
2. Open the overlay.
3. Verify that the graph has exactly the same position, width and height.
4. Verify that the overlay visually covers part of the graph.
5. Verify that the graph is not moved into a separate column.
6. Close the overlay.
7. Verify that the original Results layout is restored without artifacts.
8. Repeat after resizing the terminal.

**If opening a dialog changes the graph dimensions, the implementation does not meet this specification.**

### 10. Non-Goals

Do not redesign the graph renderer, query editor or Prometheus API integration as part of this change.

This task concerns overlay positioning, compositing, focus management and rendering correctness.
