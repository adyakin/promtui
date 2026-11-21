# PromTUI — Milestone 9: Alertmanager Silence Management

## Objective

Implement full Alertmanager silence management in PromTUI, using Alertmanager API v2.

Users must be able to:

1. View existing silences, including active, pending and expired.
2. Inspect silence details and see which alerts are affected.
3. Create silences with editable label matchers and an affected-alert preview.
4. Edit existing silences.
5. Expire existing silences with confirmation.
6. Create a silence directly from an alert's Details overlay, with matchers prefilled from that alert.

Introduce a dedicated **Silences** tab rather than embedding silence management inside the existing Alerts tab.

The implementation must follow PromTUI's existing Bubble Tea architecture, keyboard conventions, visual style, and overlay behavior.

Before implementation, inspect the current codebase and all `docs/uix*.md` files. Reuse existing UI components, API clients, models, and patterns wherever possible.

---

## 1. Application Tabs

Change the application tab order to:

```text
0 PromQL | 1 Alerts | 2 Silences | 3 Targets
```

Requirements:

- Add a dedicated Silences tab at index `2`.
- Move Targets from index `2` to `3`.
- Preserve existing PromQL, Alerts, and Targets functionality.
- Update keyboard shortcuts for tab switching.
- Update tab labels, footer hints, documentation and tests.
- Preserve independent per-tab state.
- Preserve existing Kitty graphics cleanup and focus behavior when switching tabs.
- If the application supports alternative tab-switching shortcuts such as `Alt+number`, extend them consistently.

Silences must have its own table, selection, search, filters, refresh lifecycle, loading state and error state.

Do not implement Silences as a subview of Alerts.

---

## 2. Alertmanager API Integration

Use the existing per-connection `alertmanager_url` configuration.

All silence operations must use **Alertmanager API v2**.

Required endpoints:

| Operation | Endpoint |
|---|---|
| List silences | `GET /api/v2/silences` |
| Get silence | `GET /api/v2/silence/{silenceID}` |
| Create silence | `POST /api/v2/silences` |
| Update silence | `POST /api/v2/silences` with existing silence ID |
| Expire silence | `DELETE /api/v2/silence/{silenceID}` |
| List alerts for preview | `GET /api/v2/alerts` |

Use the actual API schema and response models.

### Requirements

- Reuse existing Alertmanager HTTP client infrastructure where possible.
- Reuse connection-specific TLS settings, including `insecure_skip_verify`.
- Respect existing request timeout conventions.
- Handle HTTP errors and Alertmanager error responses.
- Never report a successful operation before the API confirms it.
- Avoid duplicate submissions while a request is in progress.
- Handle network failures without losing the user's editor input.
- Avoid blocking the Bubble Tea event loop during requests.
- Ensure stale asynchronous responses cannot overwrite newer state or state belonging to another connection.
- Do not add new backend mode/type configuration.

If `alertmanager_url` is absent, show an informative unavailable state rather than crashing or attempting silence operations against the metrics URL.

Silence management must remain independent of PromQL and Targets API availability.

---

## 3. Silences Tab — List View

The Silences tab displays silences returned by Alertmanager.

### Default visibility

**Show Active and Pending silences by default.**

Expired silences must be hidden initially, but users must be able to include them.

Example:

```text
2 Silences                                             1/4

3 active · 1 pending · 12 expired
Status: Active + Pending       [e] Include expired

STATUS    MATCHERS                    CREATED BY   ENDS IN
Active    servicename="billing"       andrey       1h20m
Active    alertname="HighCPU"         andrey       45m
Pending   job="node-exporter"         andrey       2d4h
```

The example is illustrative, not a pixel-perfect layout requirement.

### Columns

Initial columns:

- Status
- Matchers
- Created by
- Comment
- Ends in / End time

Use available horizontal space effectively while keeping the table readable.

Long matcher lists and comments should be truncated in the table, with full values available in Details.

Do not add YAML-configurable columns for Silences in this milestone.

### Summary

Display counts of:

- Active
- Pending
- Expired

Counts must include all fetched silences, even when expired entries are hidden.

### Expired filter

Use `e` to toggle inclusion of expired silences.

Requirements:

- Default: active and pending only.
- `e`: include expired.
- Pressing `e` again: hide expired.
- Preserve selection where possible.
- Do not reset search unnecessarily.
- Newly expired silences disappear from the default view after refresh.
- The footer must show the relevant shortcut.

### Search

Reuse PromTUI's existing fuzzy-search implementation.

Search across relevant silence fields, including:

- Matcher names and values
- Creator
- Comment
- Status

Search only among silences allowed by the current status filter.

Do not introduce a new search algorithm or query language.

### Sorting

Provide sorting consistent with the existing application conventions.

At minimum, support sorting by:

- Status
- End time
- Creator

Prefer a useful default order: active first, then pending, with soonest expiration first within each group.

Expired silences should appear after non-expired ones when included.

### Refresh

Follow existing Alerts/Targets refresh conventions.

Requirements:

- Initial fetch on opening the tab.
- Manual refresh with `r`.
- Automatic refresh using the application's existing interval, approximately 30 seconds.
- Preserve selection and search where possible.
- Handle empty results.
- Handle network errors without discarding valid previously fetched data unnecessarily.
- Update status classification as time passes, using the API status and appropriate time handling.

---

## 4. Silence Details Overlay

Press `Enter` on a silence to open its details.

This must be a **true floating overlay** over the existing table, consistent with PromTUI's current Alert Details overlay.

Do not shrink or reflow the underlying table.

Display:

- Silence ID
- Status
- Creator
- Comment
- Start time
- End time
- Remaining duration, where applicable
- Full list of matchers
- Matcher operators
- Currently affected alerts

### Actions

From the Details overlay, support:

- Edit silence
- Expire silence
- View affected alerts
- Close with `Esc`

Use contextual keyboard shortcuts and display them in the footer or overlay hints.

Do not reuse a shortcut in a way that conflicts with the active overlay's text inputs.

### Affected alerts

Show alerts matching the silence's matcher set.

Include relevant identifying information such as:

- Alert name
- Severity
- Instance, if present
- Alert state

Do not assume every alert has `instance`, `job`, or `severity`.

Support scrolling through long results.

The overlay must remain usable with many matchers or matching alerts.

---

## 5. Shared Silence Editor

Implement one reusable editor for:

- Creating a new silence from the Silences tab.
- Creating a silence from an existing alert.
- Editing an existing silence.

Use a large floating overlay optimized for terminals with more horizontal than vertical space.

The editor must not replace or reflow the underlying tab.

### Layout

Prefer a compact, horizontally organized layout when sufficient width is available.

Illustrative example:

```text
┌─ New silence ─────────────────────────────────────────────────────────────┐
│                                                                          │
│ Start: Now                       Expires: Duration 2h                    │
│ Created by: andrey               Comment: Maintenance                    │
│                                                                          │
│ Matchers                                                                 │
│ alertname = AWGQuickServiceDown     severity = critical                  │
│ instance  = 192.168.20.1           job      = nodes                     │
│ name      = awg-quick@awg0.service                                      │
│                                                                          │
│ Matching alerts: 1                                                       │
│                                                                          │
│ Tab Next  p Preview  Ctrl+Enter Create  Esc Cancel                       │
└──────────────────────────────────────────────────────────────────────────┘
```

This is conceptual. Follow existing UI patterns rather than implementing this layout literally.

Requirements:

- Responsive to terminal dimensions.
- Compact on wide terminals.
- Scrollable when content exceeds available height.
- No clipped controls or inaccessible fields.
- Clear focus indication.
- Keyboard-only operation.
- Preserve underlying view and selection.
- Preserve editor values when previewing or encountering API errors.

---

## 6. Start Time and Expiration

Support both relative duration and absolute date/time input.

### Start time

Support:

- `Now` — default.
- Explicit future start time for scheduled silences.

### Expiration mode

Support two alternatives:

**Duration**

Examples:

```text
30m
2h
24h
7d
```

**Until date**

Example:

```text
2026-11-25 18:30
```

The absolute date/time field must show a clear format placeholder:

```text
YYYY-MM-DD HH:MM
```

### Timezone

Interpret entered absolute times in the user's local timezone.

Display the resolved timezone or UTC offset in the editor.

Convert times to the format expected by Alertmanager API v2.

Avoid ambiguous or silently incorrect timezone conversions.

### Validation

Reject:

- Invalid date/time formats.
- Invalid durations.
- Zero or negative durations.
- End times before or equal to start times.
- End times already in the past when creating an immediately active silence.
- Ambiguous or nonexistent local times during daylight-saving transitions, unless the UI explicitly resolves the ambiguity.

Show actionable validation errors without closing the editor.

### Duration parsing

Support Go-style durations and convenient day notation such as `7d`.

Go's standard `time.ParseDuration` does not support `d`, so implement this explicitly.

Define `d` as a fixed 24-hour duration and document that behavior.

### API representation

Submit explicit `startsAt` and `endsAt` timestamps.

For `Now`, resolve the start timestamp at submission time so a long-open editor does not create an unexpectedly shortened silence.

---

## 7. Matcher Editor

Support Alertmanager matcher operators:

```text
=
!=
=~
!~
```

Each matcher consists of:

- Label name
- Operator
- Value

Requirements:

- Add a matcher.
- Edit an existing matcher.
- Remove a matcher.
- Navigate between matchers using the keyboard.
- Preserve matcher order in the editor.
- Validate matcher syntax.
- Validate regular expressions for regex operators.
- Require at least one matcher.
- Preserve exact label values, including spaces and special characters.

Do not restrict matchers to a predefined set of labels.

### Safety

Warn before creating a silence with unusually broad matchers, especially matchers that can match missing labels or empty values.

Do not silently modify user-provided matchers.

---

## 8. Create Silence from Selected Alert

The existing Alerts list uses `s` for sorting.

**Do not change that shortcut.**

Instead, add a contextual `s` shortcut to the existing **Alert Details overlay**.

Workflow:

```text
Alerts tab
    ↓
Select alert
    ↓
Enter — Alert Details
    ↓
s — Silence this alert
    ↓
New Silence editor
    ↓
Review prefilled matchers
    ↓
Preview affected alerts
    ↓
Create silence
```

### Prefilled matchers

Populate the editor with all labels from the selected alert.

For example:

```text
alertname = AWGQuickServiceDown
instance  = 192.168.20.1
job       = nodes
name      = awg-quick@awg0.service
severity  = critical
state     = active
type      = oneshot
```

Use exact equality matchers for all prefilled labels.

All matchers must initially be enabled.

Users can remove or modify matchers before creating the silence.

### Important distinction

Only use labels from the alert's actual `labels` map.

Do not accidentally include derived status fields such as `status.state`.

If an alert genuinely has a label named `state`, it is a valid matcher.

### Navigation

- `s` in Alerts list: existing Sort behavior.
- `s` in Alert Details: open prefilled silence editor.
- `Esc` from silence editor: return to Alert Details with its content and scroll position preserved.
- Successful creation: return to Alerts and refresh alert statuses.

Update contextual footer hints accordingly.

---

## 9. Affected Alerts Preview

Before submitting a silence, users must be able to inspect which currently known alerts match the proposed matcher set.

### Behavior

The preview must update based on the editor's current matcher definitions.

Support an explicit `p` shortcut to open the preview.

The editor should also display the current matching-alert count when a valid preview is available.

### Matching semantics

Implement matching consistent with Alertmanager:

- `=` exact equality
- `!=` inequality
- `=~` regex match
- `!~` negative regex match
- All matchers combined with logical AND
- Correct behavior for missing labels and empty values
- Regex semantics consistent with Alertmanager, including full-string matching

Do not implement a simplified matcher that can disagree with Alertmanager on common cases.

Prefer reusing an appropriate existing Prometheus/Alertmanager matcher implementation rather than building an incompatible custom parser.

### Alert source

Use Alertmanager API v2 alerts.

Include currently active, silenced and inhibited alerts where returned by the API.

Do not restrict the preview to unsilenced alerts.

The preview must not depend on the current Alerts tab's visible rows, search or filters.

### Preview display

Show:

- Number of matching alerts
- Alert name
- Relevant labels
- Current alert status

Allow scrolling through the matching alerts.

Clearly distinguish:

- No alerts match
- Preview still loading
- Preview failed
- Preview results available

### Accuracy

Preview results represent currently fetched alerts, not future alerts.

A silence can affect future alerts with matching labels.

Make this distinction clear in the UI.

### Stale results

If matchers change, invalidate the previous preview count immediately.

Do not display an old count as though it applies to the new matcher set.

Prevent stale asynchronous preview results from replacing newer results.

### Submission

Users must be able to inspect the preview before confirming creation.

A failed preview must not silently be represented as zero matching alerts.

Do not create or update a silence merely by opening the preview.

---

## 10. Creating a Silence

From the Silences list, provide `n` for New Silence.

The editor must support:

- Start time
- Duration or absolute end time
- Matchers
- Creator
- Comment
- Affected-alert preview

### Creator

Add a global optional YAML setting:

```yaml
silence_creator: andrey
```

Use it as the default value in the editor.

The field must remain editable.

Do not hardcode a creator name.

If the setting is absent, use an empty field and require the user to provide a creator before submission.

### Comment

Require a non-empty comment, consistent with Alertmanager's silence creation workflow.

### Submission

Use `Ctrl+Enter` to submit from the editor.

Requirements:

- Validate before sending.
- Show a pending/submitting state.
- Prevent duplicate submissions.
- Show API errors without losing editor contents.
- Close the editor only after confirmed success.
- Refresh the Silences list after success.
- Refresh Alerts when relevant.
- Preserve application responsiveness.

---

## 11. Editing Existing Silences

From Silence Details, provide an Edit action.

Open the same editor populated with the existing silence's:

- Matchers
- Start time
- End time
- Creator
- Comment

Preserve the existing silence ID when submitting an update.

Use Alertmanager API v2 update semantics.

### Requirements

- Clearly distinguish Create and Save Changes actions.
- Preserve unchanged values.
- Validate edited values.
- Show API errors without losing edits.
- Refresh the list and details after successful update.
- Do not create an unintended duplicate silence.

### Expired silences

Do not silently reactivate expired silences.

For an expired silence, offer **Create new from this silence** instead of modifying its historical record.

This creates a new silence draft with copied matchers, creator and comment, but a new ID and newly selected start/end times.

---

## 12. Expiring Existing Silences

Alertmanager does not use ordinary permanent deletion for silence management.

Implement **Expire Silence** using:

```http
DELETE /api/v2/silence/{silenceID}
```

Use the label **Expire**, not Delete, in the UI.

### Requirements

- Require explicit confirmation.
- Show identifying information in the confirmation dialog.
- Never expire a silence merely by pressing a key without confirmation.
- Prevent repeated requests while expiration is pending.
- Handle API errors.
- Refresh the Silences list after success.
- Refresh Alerts so updated silence status can be reflected.
- Preserve selection where possible.

Expired silences should disappear from the default list after a successful operation.

They must remain accessible when the user enables expired entries.

Do not offer Expire on an already expired silence.

---

## 13. Keyboard Shortcuts

Follow existing application conventions.

Suggested shortcuts:

| Context | Key | Action |
|---|---|---|
| Global | `2` | Silences tab |
| Global | `3` | Targets tab |
| Silences list | `n` | New silence |
| Silences list | `Enter` | Details |
| Silences list | `e` | Include/exclude expired |
| Silences list | `/` | Fuzzy search |
| Silences list | `r` | Refresh |
| Silences list | `s` | Sort, if consistent with existing views |
| Alert Details | `s` | Silence selected alert |
| Silence Details | `e` | Edit |
| Silence Details | `x` | Expire |
| Silence Details | `a` | Affected alerts |
| Silence editor | `Tab` / `Shift+Tab` | Navigate fields |
| Silence editor | `p` | Preview, when not editing text |
| Silence editor | `Ctrl+Enter` | Create / Save |
| Overlays | `Esc` | Cancel / Back / Close |

### Shortcut conflicts

The table above is a proposed mapping.

Inspect existing keybindings before implementation.

Do not break existing shortcuts.

In particular:

- Alerts list `s` must remain Sort.
- Alert Details `s` must initiate silence creation.
- Text input must receive normal typed characters.
- Global tab shortcuts must not interfere with editing text.

The active overlay or editor must take precedence over underlying list shortcuts.

---

## 14. Overlay and Navigation Behavior

All new overlays must follow existing PromTUI UX conventions.

Requirements:

- True floating overlays.
- No resizing or reflowing the underlying table.
- Clear active borders.
- Contextual footer hints.
- Consistent keyboard navigation.
- `Esc` returns to the previous context.
- Preserve underlying selection and scroll state.
- Avoid losing partially entered values.
- Support scrolling for long content.
- Handle terminal resizing gracefully.
- Never let keyboard events leak into the underlying view while an overlay is active.

Follow all applicable `docs/uix*.md` specifications.

---

## 15. Connection Switching

Silences belong to the currently selected connection's Alertmanager.

Requirements:

- Fetch silences from the selected connection's `alertmanager_url`.
- Never mix silences from different connections.
- Clear or invalidate stale request results on connection switching.
- Preserve appropriate per-connection or per-tab state according to existing conventions.
- Do not accidentally submit changes to a different connection if the connection changes while an editor is open.

For an open editor, either lock the operation to its original connection or safely cancel/invalidate it when switching connections.

Choose the approach that best fits the existing application architecture, and document the decision.

---

## 16. Error Handling

Handle at least:

- Alertmanager unavailable.
- Missing `alertmanager_url`.
- Authentication or authorization errors.
- Invalid matcher expressions.
- Invalid start/end times.
- Silence not found.
- Silence already expired.
- Concurrent modification or stale silence data.
- API timeout.
- Partial or failed alert preview.
- Failed creation, editing or expiration.

Errors must be actionable and visible in the TUI.

Preserve editor contents after recoverable failures.

Do not display stale success states after a failed operation.

---

## 17. Testing

### API

- List silences.
- Retrieve silence details.
- Create silence.
- Update existing silence.
- Expire silence.
- Handle API failures.
- Verify connection-specific TLS configuration.
- Verify requests use the correct Alertmanager base URL.
- Verify stale responses are ignored.

Use mocked HTTP servers for write-operation tests. Do not require a production Alertmanager instance.

### List view

- Active and pending visible by default.
- Expired hidden by default.
- `e` toggles expired visibility.
- Counts include hidden expired silences.
- Search works.
- Sorting works.
- Selection is preserved across refresh.
- Empty and error states work.
- Status transitions are handled.

### Editor

- New silence.
- Prefilled silence from alert.
- Edit existing silence.
- Create new from expired silence.
- Add/edit/remove matchers.
- All four matcher operators.
- Invalid regex validation.
- Relative duration parsing.
- Absolute date/time parsing.
- Timezone conversion.
- Future scheduled start.
- Missing creator/comment validation.
- Submission failure preserves values.
- Duplicate submissions are prevented.

### Preview

- Exact matcher.
- Negative matcher.
- Regex matcher.
- Negative regex matcher.
- Multiple matchers with AND semantics.
- Missing labels.
- Empty label values.
- Full-string regex behavior.
- Already silenced alerts included.
- No matching alerts.
- Preview API failure.
- Matcher changes invalidate stale preview.
- Stale asynchronous responses ignored.

### Navigation

- New tab order.
- Targets accessible through `3`.
- Alerts-list `s` still sorts.
- Alert Details `s` opens silence editor.
- `Esc` returns to the correct overlay.
- Keyboard input does not leak into underlying views.
- Terminal resize handling.
- Connection switching.

### Regression

- PromQL functionality unchanged.
- Alerts listing and details unchanged except for the new contextual action.
- Targets functionality unchanged except for its tab index.
- Existing fuzzy search preserved.
- Existing Alertmanager alert retrieval preserved.
- Existing Kitty graphics behavior preserved.
- Existing OSC 7501 query status behavior preserved.
- Existing configuration remains valid.

---

## 18. Documentation

Update user documentation with:

- New tab order.
- Silences tab usage.
- Active/pending/expired filtering.
- Creating silences.
- Editing silences.
- Expiring silences.
- Creating a silence from Alert Details.
- Matcher operators.
- Preview behavior and limitations.
- Duration and absolute date formats.
- Timezone behavior.
- Global `silence_creator` setting.
- Keyboard shortcuts.

Include example workflows for both direct silence creation and silencing an existing alert.

---

## 19. Parallel Development Considerations

This milestone may be implemented in a separate Git worktree while another agent works on configurable Alerts/Targets columns and search.

Avoid unnecessary changes to:

- Alerts table column definitions.
- Alerts fuzzy-search field configuration.
- Targets table rendering.
- Per-connection `alerts.columns`, `alerts.search`, `targets_view.columns`, or `targets_view.search`.

Expected shared areas include:

- Application tab registration and routing.
- Alert Details overlay keyboard handling.
- Alertmanager API client.
- Global configuration models.
- Footer/keybinding documentation.

Keep changes modular to reduce merge conflicts.

Do not modify the other feature branch or merge it automatically.

---

## 20. Non-Goals

Do not implement:

- Silence templates or presets.
- Recurring silences.
- Bulk silence creation.
- Bulk expiration.
- Permanent deletion of silence history.
- Runtime configuration of Silences table columns.
- Silence management through Prometheus APIs.
- Changes to Consul SD or vmagent.
- Notification routing or Alertmanager configuration editing.
- Inhibition-rule management.
- New authentication mechanisms.
- A new fuzzy-search algorithm.

---

## Acceptance Criteria

The milestone is complete when:

1. PromTUI has four tabs: PromQL, Alerts, Silences and Targets.
2. Silences is tab `2`; Targets is tab `3`.
3. Active and pending silences are shown by default.
4. Expired silences can be included with `e`.
5. Users can inspect full silence details.
6. Users can see alerts currently matching an existing silence.
7. Users can create silences with editable matchers.
8. Users can preview affected alerts before submission.
9. Users can specify relative durations or absolute expiration dates.
10. Users can schedule silences with future start times.
11. Users can edit existing non-expired silences.
12. Users can expire silences with confirmation.
13. Expired silences can be used as templates for new silences.
14. Pressing `s` in Alert Details opens a prefilled silence editor.
15. Pressing `s` in the Alerts list continues to sort.
16. Silence operations use Alertmanager API v2.
17. Existing configuration and unrelated functionality remain compatible.
18. Automated tests and documentation are updated.
19. All UI follows existing PromTUI UX conventions.
20. The feature is implemented independently and is ready for review and integration.
