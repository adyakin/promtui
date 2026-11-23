# PromTUI — UI Styling, Hyperlinks and Mouse Support

## Goal

Improve PromTUI's existing terminal interface with consistent semantic coloring, clickable terminal hyperlinks, and optional mouse navigation.

Preserve the keyboard-first design and ensure everything works over SSH.

Do not implement Alert → Rule navigation or rewrite Routing/Receivers details into YAML in this task.

## 1. Shared details styling

Implement lightweight reusable formatting helpers for detail panels.

Apply consistently to:

- Alert Details
- Target Details
- Rule Details
- Routing details
- Receiver details

### Visual conventions

- Section headings: theme accent, bold.
- Keys: muted or secondary accent.
- Ordinary values: normal foreground.
- Critical / DOWN / errors: error color.
- Warning / pending: warning color.
- Healthy / UP: success color.
- URLs: link color and underline.
- Missing values: dimmed.

Preserve the existing layout, spacing, scrolling and borders.

Color should communicate meaning, not introduce unnecessary visual noise.

Do not hardcode ANSI colors. Use the existing theme and Lip Gloss styles.

Support long and multiline values, including scrape errors and annotations.

## 2. Lightweight YAML highlighting

Create a reusable YAML syntax-highlighting helper supporting:

- Mapping keys.
- String values.
- Numbers.
- Booleans.
- Null values.
- Comments.
- List markers.
- Quoted strings.

Use theme-aware styles.

Do not introduce a full YAML language server, editor, or large syntax-highlighting framework.

Avoid incorrectly highlighting `:` or `#` inside quoted strings and URLs.

The highlighter should be independently testable and preserve the original YAML content.

This task only introduces the highlighting capability and applies it where YAML is already displayed.

A future task will use it for Routing/Receivers configuration and effective inheritance annotations.

## 3. OSC 8 terminal hyperlinks


Render URLs using OSC 8. Preserve terminal-native modifier-click behavior for opening links. When mouse reporting is enabled, a normal left-click on a displayed URL copies its original value using OSC 52. Handle wrapped URLs correctly, and provide unobtrusive copy feedback. Do not implement application-side browser launching.

Initial targets:

- Target Details: Scrape URL.
- Alert Details: Generator URL.
- Other existing detail URLs where appropriate.

Requirements:

- Display the original URL without modifying its destination.
- Support long URLs and terminal wrapping.
- Preserve correct styling and terminal display widths.
- Correctly terminate OSC 8 sequences so subsequent text is not accidentally linked.
- Sanitize terminal-control characters in untrusted URL values.
- Do not launch browsers or attempt to verify reachability.
- Do not assume the URL is accessible from the user's local machine.

### Copying URLs

Provide a contextual keyboard shortcut to copy the relevant URL.

Reuse the project's existing clipboard implementation where available.

If clipboard integration is unavailable on a remote SSH host, handle the limitation gracefully.

Do not introduce an external clipboard dependency without justification.

## 4. Optional mouse support

Use Bubble Tea's existing mouse event support.

Add configuration:

```yaml
ui:
  mouse: true
```

Preserve existing UI configuration and defaults.

Mouse support must be optional and keyboard navigation must remain fully functional.

### Required interactions

**Top-level tabs**

- Single-click a tab to switch to it.
- Preserve the same state transitions and cleanup as keyboard tab switching.

**Tables and lists**

- Single-click a visible row to select it.
- Double-click a row to open its existing details overlay.
- Mouse-wheel scrolling in the focused or hovered scrollable region.

**Overlays**

- Click an item in existing selection overlays.
- Double-click where the corresponding keyboard action opens details.
- Respect modal focus and prevent clicks from reaching underlying panels.

### Double-click detection

Use a reasonable threshold, approximately 300–400 ms.

A double-click must target the same logical item.

Do not treat two clicks on different rows as a double-click.

Handle scrolling and data refreshes without accidentally opening a different item.

### Terminal compatibility

- Work with ordinary SSH sessions.
- Do not require Rex-specific capabilities.
- Respect terminals that do not report mouse events.
- Avoid breaking normal terminal text selection more than necessary.
- Document how to disable mouse capture.

Mouse events must not cause unnecessary API requests or full application reloads.

## 5. Coordinate handling

Introduce a small, reusable hit-testing mechanism where useful.

Account for:

- Terminal resizing.
- Panel borders and padding.
- Table headers.
- Scrolling offsets.
- Active overlays.
- Dynamic column widths.
- Current tab and focused component.

Do not scatter hardcoded screen coordinates throughout the application.

Reuse existing layout measurements and navigation actions.

## 6. Testing

Add tests for:

- Key/value styling and semantic colors.
- Multiline values and long annotations.
- YAML highlighting of strings, comments, numbers and booleans.
- YAML values containing URLs, colons and `#`.
- OSC 8 sequence generation and termination.
- Unsafe control characters in URLs.
- Tab hit testing.
- Row selection.
- Double-click detection.
- Clicks on different rows.
- Mouse-wheel scrolling.
- Overlay event priority.
- Terminal resizing.
- Mouse disabled behavior.
- Keyboard behavior remaining unchanged.

## 7. Implementation and integration

Inspect the existing repository, UI architecture, configuration schema, and `docs/uix*.md` before editing.

Use existing patterns and shared components.

Keep implementation focused. Avoid unrelated refactoring.

Update documentation and example configuration.

Run:

```bash
go test ./...
```

Run existing formatting, linting and build checks.

Perform an independent review focusing on terminal escape safety, mouse coordinate correctness, and keyboard regressions.

## Acceptance criteria

The feature is complete when:

1. Detail panels consistently distinguish keys, values, headings and semantic states.
2. The shared YAML highlighter is available for current and future YAML views.
3. Scrape and Generator URLs are clickable through OSC 8.
4. URLs can be copied through a keyboard action where supported.
5. Mouse clicks switch tabs and select list/table rows.
6. Double-click opens existing details overlays.
7. Mouse-wheel scrolling works in relevant panels.
8. Overlays correctly capture mouse events.
9. Mouse support can be disabled.
10. All existing keyboard interactions remain functional.
11. Tests and documentation are updated.

## Explicitly out of scope

- Alert → Rule navigation.
- Routing/Receivers YAML conversion.
- Effective route inheritance resolution.
- New tabs.
- Interactive view editing.
- Changes to Alertmanager inhibition logic.
- Rewriting table sizing or saved-view configuration.
