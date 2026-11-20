# Milestone 5 — Alertmanager Integration

**Project:** PromTUI
**Status:** Ready for implementation
**Scope:** Alerts tab, Alertmanager API v2, filtering, details and refresh

## 1. Objective

Replace the existing Alerts placeholder with a functional, keyboard-driven Alertmanager alert explorer.

Users should be able to open tab `1` and immediately inspect active alerts, including silenced and inhibited alerts.

The implementation must support both Prometheus and VictoriaMetrics installations.

**Alertmanager is the source of truth for this view.** Do not query Prometheus `/api/v1/alerts` or introduce a dependency on `vmalert`.

Follow the existing architecture and UI conventions documented in:

- `docs/DESIGN.md`
- `docs/uix*.md` especcially latest `uix-layover.md`
- Existing milestone documentation

Preserve all existing PromQL and Targets functionality.

## 2. Connection Configuration

Extend the existing connection configuration with an optional Alertmanager endpoint.

Example (adapt to the repository's actual configuration format):

```yaml
connections:
  production:
    type: victoriametrics
    url: http://victoriametrics:8428
    alertmanager_url: http://alertmanager:9093

  staging:
    type: prometheus
    url: http://prometheus:9090
    alertmanager_url: http://alertmanager-staging:9093
```

Requirements:

- `alertmanager_url` is optional.
- Do not infer an Alertmanager URL from the Prometheus/VictoriaMetrics URL.
- Reuse existing HTTP client conventions, timeout handling and TLS configuration where applicable.
- Preserve backward compatibility with existing configuration files.
- Alertmanager URL must support a reverse-proxy path prefix.
- Do not log credentials or sensitive HTTP headers.

If `alertmanager_url` is absent, show a helpful configuration message inside the Alerts tab.

Do not treat missing configuration as an empty alert list.

## 3. Alertmanager API

Use Alertmanager API v2.

Primary endpoint:

```http
GET /api/v2/alerts
```

Request all current alerts, including suppressed alerts.

Use the API's `active`, `silenced`, `inhibited` and `unprocessed` query parameters explicitly where supported, rather than relying on defaults.

For the initial implementation, request:

```text
active=true
silenced=true
inhibited=true
unprocessed=true
```

Use client-side filtering and sorting for the MVP.

### Alert data

Parse and preserve:

- `fingerprint`
- `labels`
- `annotations`
- `startsAt`
- `endsAt`
- `updatedAt`
- `generatorURL`
- `receivers`
- `status.state`
- `status.silencedBy`
- `status.inhibitedBy`

Handle optional or missing fields gracefully.

### Alert identity

Use the Alertmanager fingerprint as the primary identity within an Alertmanager source.

Do not use `alertname` alone as an identifier.

If fingerprints are unavailable, derive a stable identity from the complete label set and source.

## 4. Alerts Tab Layout

Use the existing application shell and panel styling.

Do not introduce a new visual design.

The Alerts tab contains:

- Compact summary
- Scrollable alerts table
- Status information
- Context-sensitive footer

### Summary

Show counts for:

- Total alerts
- Unsuppressed active alerts
- Silenced alerts
- Inhibited alerts

Counts must be derived from actual Alertmanager response data.

Do not assume that suppression categories are mutually exclusive.

Keep the summary compact, preferably one line.

Example:

```text
Alerts: 24 total · 18 active · 4 silenced · 2 inhibited
```

### Table

Suggested columns:

| Column | Content |
|---|---|
| State | Active / Silenced / Inhibited / Unprocessed |
| Severity | `severity` label |
| Alert | `alertname` label |
| Instance | `instance` label |
| Duration | Time since `startsAt` |

If a label is missing, display `—`.

Avoid displaying all labels directly in the table.

The table must support scrolling and terminals with limited width.

### Sorting

Default order:

1. Unsuppressed active alerts
2. Silenced alerts
3. Inhibited alerts
4. Unprocessed alerts

Within each group, sort by severity and then start time.

Do not assume arbitrary severity labels have a universal ranking. Recognize common values such as `critical`, `warning` and `info`, with deterministic handling for unknown values.

### Selection

- The selected row must be clearly highlighted.
- Selection must remain visible when scrolling.
- Preserve the selected alert across refreshes using its stable identity.
- If the selected alert disappears, select the nearest remaining row.
- Empty results must have a clear empty state.

## 5. Keyboard Navigation

Preserve the existing top-level tab navigation.

Inside Alerts:

| Key | Action |
|---|---|
| `↑ / ↓` | Select alert |
| `PgUp / PgDn` | Scroll table |
| `Home / End` | First / last alert |
| `Enter` | Open alert details |
| `/` | Filter alerts |
| `r` | Refresh alerts |
| `s` | Change sort order |
| `Esc` | Close active overlay |
| `?` | Context-sensitive help |
| `q` | Quit when appropriate |

Global tab switching and application shortcuts must continue to work.

Do not interfere with the existing PromQL editor.

### Context-sensitive footer

Preserve the existing footer behavior.

Example:

```text
↑↓ Select   Enter Details   / Filter   s Sort   r Refresh   ? Help
```

Footer actions must match actual keyboard behavior.

## 6. Filtering

Press `/` to enter filter mode.

Requirements:

- Case-insensitive fuzzy matching: typed characters must occur in order within a field.
- Search only the table fields: state, severity, alert name, instance, and duration.
- Labels and annotations shown only in alert details are not searched.
- Update results interactively.
- `Enter` applies the filter.
- `Esc` cancels filter editing.
- Display the current filter when active.
- Preserve filter state when switching tabs.

Filtering must operate on cached alert data without additional API requests.

Do not implement a complex PromQL-like filtering language in this milestone.

## 7. Alert Details Overlay

Press `Enter` on a selected alert to open a **true floating overlay**.

Follow the overlay requirements in `docs/uix.md`.

The overlay must not resize or replace the underlying Alerts table.

### Contents

Display:

**Overview**
- Alert name
- Severity
- State
- Active since
- Duration
- Last update
- Evaluation value, if available

**Annotations**
- Summary
- Description
- Other annotation fields

**Labels**
- Complete label set
- Sorted by label name

**Alertmanager details**
- Receivers
- Silence IDs
- Inhibition source references
- Generator URL, if present

Do not invent an evaluation value if Alertmanager does not provide one.

### Interaction

- `↑ / ↓` scroll details.
- `PgUp / PgDn` scroll one page.
- `y` copies complete labels.
- `Esc` closes the overlay.

Long annotations and labels must wrap or scroll correctly.

The overlay should be sized according to terminal dimensions, with reasonable margins.

Do not open a separate full-screen view.

## 8. Refresh Behavior

### Manual refresh

`r` refreshes the Alerts tab immediately.

Show loading state without clearing existing data.

### Automatic refresh

Default interval: **30 seconds**.

Requirements:

- Refresh automatically while the Alerts tab is active.
- Do not create duplicate refresh loops.
- Do not overlap requests unnecessarily.
- Cancel or ignore obsolete requests.
- Preserve filters, sorting and selection.
- Keep an open details overlay on successful refreshes; close it on failure to show the error.
- Avoid unnecessary visual flickering.

When returning to Alerts after visiting another tab, refresh if cached data is stale.

### Error handling

If refresh fails:

- Preserve the last successful alert list in the cache.
- Replace the visible list with the wrapped error in the Alerts pane. Let users scroll long errors.
- Allow manual retry.

A successful response containing zero alerts must display:

```text
No active alerts
```

Distinguish this from configuration errors, API failures and authentication failures.

## 9. Application Architecture

Suggested conceptual structure:

```text
Alerts View
├── Alertmanager API Client
├── Alert Data Model
├── Refresh Controller
├── Filtering / Sorting
├── Alerts Table
└── Details Overlay
```

Reuse existing infrastructure where appropriate.

Keep Alertmanager-specific API models separate from UI presentation models.

Do not introduce a large generic monitoring backend abstraction.

### Backend independence

The Alerts tab must not depend on whether the current query connection is Prometheus or VictoriaMetrics.

The only required configuration for Alerts is a reachable Alertmanager endpoint.

## 10. Testing

### Unit tests

Cover:

- Alertmanager response decoding
- Missing optional fields
- Alert identity
- Status classification
- Sorting
- Filtering
- Duration formatting
- Selection preservation
- Summary counts

### HTTP integration tests

Use a mock Alertmanager API server.

Test:

- Successful response
- Empty alert list
- HTTP errors
- Malformed JSON
- Request timeout
- Cancellation
- Authentication failure
- Reverse-proxy path prefix

### UI tests

Verify:

- Alerts tab navigation
- Selection and scrolling
- Filter behavior
- Details overlay
- Context-sensitive footer
- State preservation across tab changes
- Refresh without losing selection

### Manual Rex verification

1. Configure a real Alertmanager endpoint.
2. Open tab `1`.
3. Verify alerts are displayed.
4. Navigate the table.
5. Open and close details.
6. Filter by alert name or instance.
7. Refresh manually.
8. Wait for automatic refresh.
9. Switch to PromQL and back.
10. Verify state preservation and correct rendering.

## 11. Acceptance Criteria

Milestone 5 is complete when:

- Alerts are loaded from a configured Alertmanager API v2 endpoint.
- Both Prometheus and VictoriaMetrics connections can use the feature.
- Active, silenced and inhibited alerts are represented correctly.
- The table supports keyboard navigation, filtering and sorting.
- Alert details appear in a floating overlay.
- The selected alert remains stable across refreshes.
- Automatic refresh works without duplicate requests.
- Errors are distinguishable from empty results.
- Existing PromQL and Targets tabs remain functional.
- The context-sensitive footer remains accurate.
- No unrelated functionality is introduced.

## 12. Non-Goals

Do not implement:

- Creating, editing or deleting silences
- Alert acknowledgements
- Alertmanager routing configuration
- Notification management
- Alert grouping by routing rules
- Prometheus alert rules API
- vmalert integration
- Pending alerts
- Alert history
- Multiple Alertmanager aggregation
- Additional top-level tabs

These may be considered in future milestones.

## 13. Documentation and Handoff

After implementation, create:

`docs/MILESTONE-5.md`

Include:

- Implemented features
- API and configuration decisions
- Relevant source files
- Tests and results
- Manual verification status
- Known limitations
- Deferred features
- Recommendations for the next milestone

Distinguish automated test results from features requiring manual Rex verification.

Do not automatically proceed to Milestone 6.
