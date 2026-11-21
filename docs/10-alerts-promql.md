# PromTUI — Load Alert Generator Query into PromQL Editor

## Objective

Add the ability to extract a PromQL expression from an alert's Prometheus `generatorURL` and load it into PromTUI's existing PromQL editor.

This feature connects the Alerts and PromQL tabs, allowing users to investigate the query responsible for firing an alert without manually copying it from a browser.

**Scope:** Prometheus generator URLs only. VictoriaMetrics/vmalert URL support is deferred.

## 1. User Workflow

```text
1 Alerts
  ↓
Select an alert
  ↓
Enter — Alert Details
  ↓
p — Load in PromQL
  ↓
0 PromQL
  ↓
Query editor contains extracted expression
```

The query must be loaded but **not executed automatically**.

The user can inspect, edit and execute it using the existing `Ctrl+Enter` shortcut.

## 2. Keyboard Shortcut

Add `p` to the existing Alert Details overlay.

- `p` — Load generator query in PromQL.
- `Esc` — Close Alert Details.
- Preserve all existing shortcuts.
- Do not use `q`, which is PromTUI's global quit shortcut.
- The Alert Details overlay must handle `p` before the event reaches the underlying Alerts view.

Update the contextual footer:

```text
↑↓ Scroll   p Load in PromQL   Esc Close
```

If a future Silence Management milestone adds `s` to Alert Details, both actions must coexist without conflicts.

## 3. Prometheus URL Parsing

Read the selected alert's `generatorURL` from the existing Alertmanager API v2 model.

Support standard Prometheus graph URL query parameters.

Examples:

```text
https://prometheus.example.com/graph?g0.expr=up%7Bjob%3D%22nodes%22%7D&g0.tab=1

https://prometheus.example.com/graph?expr=up%7Bjob%3D%22nodes%22%7D
```

Both must extract:

```promql
up{job="nodes"}
```

### Requirements

- Parse URLs using Go's standard `net/url` package.
- Support `g0.expr` and `expr`.
- Prefer `g0.expr` when both are present.
- Decode URL-encoded expressions correctly.
- Preserve whitespace, newlines, quotes and special characters.
- Do not double-decode URL parameters.
- Reject empty expressions.
- Do not attempt to interpret arbitrary URL paths as PromQL.
- Do not fetch the URL or open a browser.

If the generator URL is missing, malformed or unsupported, show a concise error and leave the existing editor unchanged.

Do not add VictoriaMetrics/vmalert-specific parsing in this task.

## 4. Loading into PromQL

On successful extraction:

1. Close Alert Details.
2. Switch to tab `0 PromQL`.
3. Replace the existing editor expression with the extracted PromQL.
4. Focus the query editor.
5. Preserve normal editor behavior, including syntax highlighting, multiline editing and autocomplete.

### Important

Do not automatically execute the query.

Do not automatically change:

- Selected connection
- Instant/range query mode
- Time range
- Query history
- Saved Queries

Do not clear existing graph/table results merely because the editor expression changed.

The next explicit execution should use the newly loaded expression.

## 5. Existing Editor State

If the PromQL editor already contains text, replace it with the extracted expression.

No confirmation dialog is required.

Use the existing editor APIs or state-management mechanisms rather than simulating keyboard input.

Ensure that editor cursor position, highlighting and internal text buffers remain consistent after programmatic replacement.

## 6. Alert Details UI

Keep the existing Alert Details layout.

If a valid Prometheus query is available, display the `p` action in the contextual footer.

If the generator URL is missing or unsupported, the action may remain visible but must produce a useful error rather than doing nothing.

Do not introduce a new overlay or change the underlying Alerts table.

## 7. Testing

### URL parsing

Test:

- Standard `g0.expr` URL.
- `expr` parameter.
- Both parameters present.
- URL-encoded label selectors.
- Multiline PromQL.
- Expressions containing `+`, `%`, `&`, `=`, quotes and parentheses.
- Empty expression.
- Missing expression parameter.
- Missing generator URL.
- Malformed URL.
- Unsupported URL format.

### Navigation and state

Test:

- `p` works in Alert Details.
- `p` does not affect the Alerts list.
- Existing `q` quit behavior remains unchanged.
- Successful extraction switches to PromQL.
- Editor receives the exact decoded expression.
- Editor receives focus.
- Query is not automatically executed.
- Existing results are preserved.
- Current connection, mode and time range remain unchanged.
- Failed extraction does not overwrite the editor or change tabs.

## 8. Non-Goals

Do not implement:

- VictoriaMetrics/vmalert URL formats.
- Automatic query execution.
- Opening generator URLs in a browser.
- Changing connections based on generator URL hostname.
- Importing alert rule definitions.
- Modifying alert rules.
- Creating Saved Queries automatically.
- New keyboard shortcuts outside Alert Details.
- Changes to the Alerts table or search implementation.

## Acceptance Criteria

The task is complete when:

1. Pressing `p` in Alert Details extracts PromQL from a supported Prometheus generator URL.
2. PromTUI switches to tab `0` and populates the editor.
3. The query is not automatically executed.
4. Existing editor settings and query results are preserved.
5. Invalid or unsupported URLs produce useful errors without changing editor state.
6. Existing keyboard shortcuts and functionality remain unchanged.
7. Tests and documentation are updated.
