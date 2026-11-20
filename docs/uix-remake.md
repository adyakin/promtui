## Focus Navigation — Updated Decision

Tab is the primary navigation key between application panels.

- `Tab` cycles between Query Editor and Results.
- `Shift+Tab` cycles in the opposite direction.
- With two panels, both keys effectively toggle focus.
- The currently focused panel must have a clearly highlighted border and title.
- Executing a query preserves the current focus.
- When returning to Query, restore the previous cursor position.
- When returning to Results, preserve the selected series, scroll position and active Table/Graph view.

### Overlays

Overlays capture focus and temporarily suspend main-panel navigation.

- Autocomplete: `Tab` accepts the selected suggestion.
- Series Selector: `Tab` does not switch to Query or Results.
- History and other overlays: `Tab` follows overlay-specific behavior.
- `Esc` closes the overlay and restores the previous panel focus.

### Escape

`Esc` means Cancel, Back or Close. It must not be used to cycle between main panels.

### Footer

The context-sensitive footer must advertise `Tab` as the navigation shortcut in both panels.

Query:

`Ctrl+Enter Run   Tab Results   Ctrl+R History   Ctrl+Space Complete`

Results:

`Tab Query   g Table/Graph   s Series   r Range   q Quit`

### Acceptance Criteria

Pressing Tab repeatedly alternates predictably between Query and Results without modifying query text or losing the current selection.

Focus changes are immediately visible through the active panel border and title.
