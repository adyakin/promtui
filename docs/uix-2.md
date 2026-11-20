## Visual Implementation Requirements — Mandatory

The current implementation does not meet the intended visual design. Previous changes focused on keyboard behavior without substantially improving layout.

Use the reference screenshots in `docs/references/` as visual guidance.

### Panel borders

Implement actual rectangular panel borders around Query and Results, including left, right, top and bottom edges.

- Active panel: accent-colored border and title.
- Inactive panel: subdued border and title.
- Switching focus with Tab must visibly move the active border.
- Do not substitute horizontal separators for complete panel borders.
- Avoid doubled borders between adjacent panels.

### Overlay dialogs

Series Selector and Time Range must be floating dialogs over the existing interface.

They must NOT replace the entire application screen.

- Series Selector: right-aligned over the Results area, approximately 40% of terminal width.
- Time Range: compact centered dialog, sized to its content.
- Dialog height must depend on the number of items, with a reasonable maximum.
- Dialogs must have complete rectangular borders.
- The main screen remains visible behind the dialog.
- The dialog must have a highlighted title and selected row.
- Closing a dialog must restore the underlying interface correctly.

For Kitty graphics, explicitly handle image placement and cleanup so images cannot overwrite dialogs.

### Series Selector

Display the actual graph-series colors alongside each series.

Use visible checkboxes and a highlighted selected row.

Space toggles exactly the selected series, and the graph updates immediately.

### Footer

Replace the current light-background shortcut bar with a visually consistent footer.

- Use the same background as the application.
- Highlight shortcut names using an accent foreground color or bold text.
- Do not use filled rectangular backgrounds around individual keys.
- Do not leave light-colored blocks at the beginning or end of the footer.
- Render the footer across the full terminal width.
- Preserve context-sensitive shortcuts.
- Prevent wrapping or clipping from corrupting the layout.

### Visual verification

Before declaring completion, compare the running application with the reference screenshots.

Verify:
1. Complete borders around both panels.
2. Obvious active-panel highlighting.
3. Compact floating dialogs, not full-screen replacement views.
4. Consistent footer background with no stray color blocks.
5. Series colors matching the graph.
6. No Kitty image artifacts when opening or closing dialogs.

If the agent cannot visually inspect the actual Rex terminal, it must explicitly request manual verification rather than claim visual success.
