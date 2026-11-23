# PromTUI — Milestone 10: Alertmanager Routing Explorer

> This is the original milestone specification. For current receiver YAML
> display, filtering, and clipboard behavior, see [Exploring routes and receivers](routing.md).

## Objective

Implement a read-only Alertmanager routing explorer in PromTUI.

The feature must allow users to:

1. Browse the complete Alertmanager routing tree.
2. Inspect route matchers, grouping rules, timing and inheritance.
3. See which receiver handles notifications for a selected route.
4. Inspect a receiver's configured notification integrations and destinations.
5. Navigate from a route to its receiver.
6. Navigate from a receiver to every route that references it.
7. Switch between dedicated Routes and Receivers views.

The goal is to make Alertmanager routing configuration understandable directly from the terminal without opening its web interface or manually reading YAML.

**This milestone is strictly read-only.**

Before implementation, inspect the existing application architecture, configuration models, Alertmanager API client, tab navigation, overlays and all `docs/uix*.md` files.

Reuse existing components and conventions wherever possible.

---

## 1. Application Tab

Add a dedicated Routing tab.

Assuming the Silence Management milestone is already integrated, the application should have:

```text
0 PromQL | 1 Alerts | 2 Silences | 3 Targets | 4 Routing
```

Use index `4` for Routing.

If the actual tab layout differs when implementation begins, inspect the current codebase and integrate without removing or overwriting existing tabs.

### Requirements

- Routing has independent state, selection, loading and error handling.
- Switching tabs preserves Routing navigation state.
- Existing keyboard shortcuts continue working.
- Existing Kitty graphics cleanup behavior is preserved.
- Update tab labels, footer hints, tests and documentation.
- Do not introduce a separate top-level Receivers tab.

## 2. Two Views Within Routing

The Routing tab contains two views:

- **Routes** — routing hierarchy and route details.
- **Receivers** — configured receivers and their notification integrations.

Allow switching with `v`, following the application's contextual keyboard conventions.

Preserve each view's selection and scroll position when switching.

### Routes view

Use a two-panel layout:

```text
┌─ Routes ───────────────────────────┬─ Route details ────────────────────────────┐
│                                   │                                           │
│ ▼ root → default                  │ Matchers: severity="critical"              │
│   ├─ severity="critical"           │ Receiver: pagerduty                        │
│   │  └─ team="payments"            │ Continue: false                            │
│   ├─ servicename="billing"         │ Group by: alertname, instance              │
│   └─ team="infra"                  │ Group wait: 30s                            │
│                                   │ Group interval: 5m                         │
│                                   │ Repeat interval: 4h                        │
│                                   │                                           │
│                                   │ Receiver integrations:                    │
│                                   │   PagerDuty × 1                           │
│                                   │                                           │
│                                   │ Enter → Receiver details                  │
└───────────────────────────────────┴───────────────────────────────────────────┘
```

### Receivers view

Use a receiver list and details panel:

```text
┌─ Receivers ────────────────────────┬─ Receiver details ─────────────────────────┐
│                                   │                                           │
│ default                           │ Name: telegram                             │
│ telegram                          │ Integrations: Telegram × 1                 │
│ payments-team                     │ Destination: chat_id -100...               │
│ pagerduty                         │                                           │
│                                   │ Used by routes:                            │
│                                   │   root → team="infra"                      │
│                                   │   root → severity="warning"                │
│                                   │                                           │
│                                   │ Enter → Referencing routes                │
└───────────────────────────────────┴───────────────────────────────────────────┘
```

These are conceptual layouts, not pixel-perfect requirements.

Prefer efficient horizontal use of terminal space.

On narrow terminals, adapt the layout gracefully without making content inaccessible.

---

## 3. Data Source

Use the existing per-connection:

```yaml
connections:
  production:
    url: https://victoriametrics.example.com
    alertmanager_url: https://alertmanager.example.com
```

Fetch Alertmanager runtime configuration through:

```http
GET /api/v2/status
```

The API response contains `config.original`, representing the Alertmanager configuration in YAML form.

### Requirements

- Use Alertmanager API v2.
- Reuse the existing Alertmanager HTTP client.
- Respect connection-specific TLS configuration.
- Respect existing timeouts.
- Parse the returned YAML locally.
- Do not require access to Alertmanager configuration files on disk.
- Do not use Prometheus or VictoriaMetrics APIs for routing configuration.
- Do not introduce additional connection settings.

If `alertmanager_url` is absent, display an informative unavailable state.

### Configuration parsing

Prefer Alertmanager's official configuration types and parsing semantics when practical.

Do not assume that every field is present in the YAML.

Handle supported Alertmanager configuration structures, including:

- Root route
- Nested routes
- Matchers
- Receiver assignments
- `continue`
- `group_by`
- `group_wait`
- `group_interval`
- `repeat_interval`
- Receiver integration configurations

Support modern `matchers` syntax and legacy `match` / `match_re` syntax where supported by the installed Alertmanager version.

Do not assume all Alertmanager installations use identical configuration versions.

---

## 4. Routes View — Hierarchical Navigation

Display the actual routing tree.

Requirements:

- Show the root route.
- Preserve parent-child relationships.
- Preserve sibling ordering from the configuration.
- Support expanding and collapsing branches.
- Indicate expanded and collapsed nodes.
- Show matcher summaries.
- Show the configured or effective receiver.
- Highlight the selected route.
- Support scrolling for large trees.

### Navigation

Suggested keys:

| Key | Action |
|---|---|
| `j` / `↓` | Next visible route |
| `k` / `↑` | Previous visible route |
| `h` / `←` | Collapse / navigate toward parent |
| `l` / `→` | Expand route |
| `Enter` | Open selected route's receiver details |
| `v` | Switch to Receivers view |
| `r` | Refresh configuration |
| `/` | Search routes |

Preserve existing keybinding conventions if they differ.

### Route identity

Routes do not have stable IDs in the configuration.

Assign internal identities based on their position in the routing tree.

Do not rely on matcher text being unique.

Two routes may have identical matchers but different positions or settings.

Preserve selection across refresh when possible.

---

## 5. Route Details

Show full information for the selected route.

### Matchers

Display:

- Matcher label
- Operator
- Value

Support:

```text
=
!=
=~
!~
```

Multiple matchers are combined according to Alertmanager routing semantics.

Display the root route as the default catch-all route.

### Receiver

Display the route's effective receiver.

If the route inherits its receiver from a parent, make this visible:

```text
Receiver: telegram (inherited from root)
```

If explicitly configured:

```text
Receiver: payments-team
```

### Grouping and timing

Display:

- Group by
- Group wait
- Group interval
- Repeat interval

Resolve inherited values according to Alertmanager configuration semantics.

For inherited fields, indicate the source:

```text
Group wait: 30s (inherited)
Repeat interval: 4h (explicit)
```

### Continue

Display the `continue` setting and explain its effect concisely.

For example:

```text
Continue: true
Matching continues to later sibling routes
```

### Important

Do not flatten the tree or incorrectly treat a route as an independent filter.

Alertmanager evaluates routes hierarchically, and sibling order and `continue` affect routing behavior.

The displayed hierarchy must preserve these semantics.

---

## 6. Route → Receiver Navigation

From a selected route, users must be able to inspect its effective receiver.

Pressing `Enter` should open receiver details for the selected route.

The receiver details must include:

- Receiver name
- Integration types
- Configured destinations
- Number of notification integrations
- Routes referencing that receiver

### Navigation behavior

- Preserve the selected route.
- Allow returning with `Esc`.
- Do not lose the routing tree's expansion state.
- Do not reset search or scroll position unnecessarily.

A route may inherit its receiver.

Navigation must use the **effective receiver**, not only the receiver explicitly specified in that route's YAML.

---

## 7. Receivers View

Display all configured receivers.

Do not limit the list to receivers referenced by routes.

Unused receivers must remain visible.

### Receiver list

Display:

- Receiver name
- Integration types
- Integration count
- Number of referencing routes

Example:

```text
RECEIVER         INTEGRATIONS           ROUTES
default          Webhook × 1            1
telegram         Telegram × 1           3
payments-team    Slack × 2              2
pagerduty        PagerDuty × 1          1
unused           Email × 1              0
```

Support:

- Selection
- Scrolling
- Fuzzy search
- Sorting where consistent with existing views
- Empty state
- Refresh

Search across receiver names and safe destination metadata.

Do not include secret values in searchable text.

---

## 8. Receiver Details

For each receiver, display all configured notification integrations.

One receiver may contain multiple integrations of the same or different types.

Examples include:

- Telegram
- Slack
- Email
- Webhook
- PagerDuty
- Opsgenie
- Microsoft Teams
- Discord
- Pushover
- Other integrations supported by the Alertmanager version

Do not hardcode an exhaustive list that breaks when new integration types appear.

Use generic fallback rendering for unknown integration types.

### Destination information

Display meaningful destination metadata where available.

Examples:

```text
Telegram
  Chat ID: -100...

Slack
  Channel: #payments-alerts

Email
  To: alerts@example.com

Webhook
  URL: https://hooks.example.com/...
```

Destination values must be derived from the actual runtime configuration.

Do not invent destinations when a field is absent.

If an integration's destination cannot be determined safely, display the integration type and available non-sensitive metadata.

### Secret handling

Alertmanager configuration can contain sensitive data.

Requirements:

- Never display API tokens, passwords, secret keys or authorization headers.
- Redact credentials embedded in URLs.
- Redact sensitive URL query parameters.
- Do not reveal sensitive webhook paths or tokens.
- Do not expose raw configuration containing secrets.
- Do not copy secrets into the clipboard.
- Do not log raw configuration or secret-bearing API responses.
- Respect values already redacted by Alertmanager.
- Treat unknown integration fields conservatively.

The purpose is to show **where notifications are configured to go**, not expose credentials.

For URLs, prefer displaying a sanitized host and safe path information rather than the entire raw URL.

If the destination is stored in a secret reference or otherwise unavailable, display a safe placeholder.

---

## 9. Receiver → Routes Navigation

This is a core requirement.

When a receiver is selected, display every route that uses it.

Include both:

- Routes that explicitly configure that receiver.
- Routes that inherit that receiver from an ancestor.

Distinguish these relationships in the UI.

Example:

```text
Receiver: telegram

Used by routes:

  root
    Explicit

  root → team="infra"
    Inherited from root

  root → severity="warning"
    Explicit
```

### Navigation

Users must be able to select a referencing route and jump directly to it in the Routes view.

When jumping:

1. Switch to Routes.
2. Expand the selected route's ancestors.
3. Select the referenced route.
4. Scroll it into view.
5. Show its details.

Preserve the Receivers view's previous selection so the user can return without losing context.

### Reference counts

Show the number of routes whose effective receiver is the selected receiver.

Do not count only explicit receiver declarations.

### Important distinction

These are **configuration references**, not proof that alerts currently reach that receiver.

A route may have no matching alerts.

Do not label these counts as delivered notifications or active alerts.

---

## 10. Search

Reuse the application's existing fuzzy-search implementation.

### Routes search

Search relevant route information:

- Matcher names
- Matcher values
- Effective receiver name

Search should work across the full routing tree, including collapsed branches.

When navigating to a search result, automatically reveal its ancestors.

Do not permanently discard hidden tree nodes because of a search.

### Receivers search

Search:

- Receiver name
- Integration types
- Safe destination metadata

Do not search credentials or secret fields.

### Requirements

- Preserve existing fuzzy matching behavior.
- Do not introduce a new search algorithm.
- Preserve search state during refresh where possible.
- Support clearing search.
- Handle empty results.

---

## 11. Refresh and Connection Switching

### Refresh

Use `r` for manual refresh.

Follow existing automatic refresh conventions if appropriate.

However, routing configuration changes much less frequently than alerts or targets.

Avoid unnecessary high-frequency polling.

A longer refresh interval or manual refresh only is acceptable if consistent with the application's architecture.

### Connection switching

Routing data belongs to the selected connection.

Requirements:

- Fetch from the current connection's Alertmanager.
- Never mix routing trees or receivers from different connections.
- Invalidate stale asynchronous responses.
- Preserve navigation state where reasonable.
- Reset invalid selections safely when configuration changes.
- Handle missing Alertmanager configuration gracefully.

### Loading and errors

Show distinct states for:

- Loading
- Configuration available
- Alertmanager unavailable
- Missing `alertmanager_url`
- Invalid configuration response
- YAML parsing failure
- No receivers configured

If refresh fails after a previous successful load, preserve the last valid configuration and show a visible stale-data/error indication.

Do not silently replace valid routing information with an empty view.

---

## 12. Internal Data Model

Build a normalized, read-only representation of the Alertmanager configuration.

Conceptually:

```go
type RouteNode struct {
    ID                string
    ParentID          string
    Children          []string
    Matchers          []Matcher
    ExplicitReceiver  string
    EffectiveReceiver string
    Continue          bool
}
```

Include effective grouping and timing settings in the actual model.

Maintain a receiver index:

```go
map[string][]RouteReference
```

Each reference should identify:

- Route ID
- Route path
- Whether the receiver is explicit or inherited

Use this index for efficient Receiver → Routes navigation.

### Requirements

- Build the model once per successful configuration fetch.
- Preserve route order.
- Preserve inheritance.
- Handle duplicate matcher expressions.
- Avoid quadratic tree traversal where unnecessary.
- Keep API parsing separate from UI rendering.
- Keep routing data immutable between refreshes where practical.

Do not build a general-purpose Alertmanager routing simulator in this milestone.

---

## 13. Keyboard and Focus Behavior

Follow existing PromTUI conventions.

Suggested shortcuts:

| Context | Key | Action |
|---|---|---|
| Global | `4` | Routing tab |
| Routing | `v` | Toggle Routes / Receivers |
| Routes | `j/k` or arrows | Navigate routes |
| Routes | `h/l` or arrows | Collapse / expand |
| Routes | `Enter` | Receiver details |
| Receivers | `j/k` or arrows | Navigate receivers |
| Receivers | `Enter` | Receiver details / referencing routes |
| Routing | `/` | Fuzzy search |
| Routing | `r` | Refresh |
| Overlay | `Esc` | Close / Back |

The exact shortcuts should be adapted to existing application conventions.

Do not break global tab switching, query editor input or overlay navigation.

Use contextual footer hints.

### Focus

- Clearly indicate the active panel.
- Preserve selection when switching views.
- Avoid unexpected focus changes after refresh.
- Ensure keyboard events do not leak through overlays.

---

## 14. UI and Layout

Follow all `docs/uix*.md` specifications.

Requirements:

- Lazygit-like bordered panels.
- Clear active selection.
- Efficient horizontal layout.
- Readable route indentation.
- Responsive panel widths.
- Scrollable details.
- Proper handling of long matchers and receiver names.
- Consistent loading, empty and error states.
- No unnecessary modal dialogs.

Use true floating overlays for expanded details when appropriate.

Do not reflow underlying panels when opening an overlay.

Prioritize a practical terminal interface over reproducing Alertmanager's web UI.

---

## 15. Testing

### API and parsing

- Successful `/api/v2/status` response.
- Missing `config.original`.
- Invalid YAML.
- Empty receivers.
- Root route only.
- Nested routes.
- Multiple sibling routes.
- Legacy and modern matcher syntax.
- Missing optional fields.
- Connection-specific TLS settings.
- API timeout and failure.
- Stale response handling.

### Inheritance

- Explicit receiver.
- Inherited receiver.
- Multiple inheritance levels.
- Explicit grouping settings.
- Inherited grouping settings.
- Explicit timing settings.
- Inherited timing settings.
- `continue` behavior.
- Duplicate matchers on different routes.

### Receiver indexing

- Receiver used by one route.
- Receiver used by multiple routes.
- Receiver inherited by descendants.
- Unused receiver.
- Receiver referenced by root.
- Correct reference counts.
- Navigation to the correct route.

### Integration display

- Single integration.
- Multiple integrations.
- Multiple integration types.
- Unknown integration type.
- Missing destination metadata.
- Secret-bearing URL.
- Redacted configuration values.
- Credentials absent from UI and searchable text.

### Navigation

- Routes ↔ Receivers switching.
- Route → Receiver.
- Receiver → Route.
- Ancestor expansion when jumping.
- Selection preservation.
- Scrolling.
- Search in collapsed branches.
- Refresh while a route is selected.
- Connection switching.
- Terminal resize.

### Regression

- PromQL unchanged.
- Alerts unchanged.
- Silences unchanged.
- Targets unchanged.
- Existing Alertmanager integration unchanged.
- Existing fuzzy search preserved.
- Existing tab shortcuts preserved.
- Kitty graphics behavior preserved.
- OSC 7501 behavior preserved.

---

## 16. Documentation

Document:

- New Routing tab.
- Routes and Receivers views.
- Navigation shortcuts.
- Routing tree expansion/collapse.
- Route details.
- Receiver details.
- Inherited settings.
- `continue` behavior.
- Receiver → Routes navigation.
- Route → Receiver navigation.
- Search and refresh.
- Security/redaction behavior.
- API requirements and limitations.

Include an example configuration demonstrating:

- Root receiver
- Nested routes
- Inherited receiver
- Multiple notification integrations
- Receiver referenced by multiple routes

---

## 17. Non-Goals

Do not implement:

- Editing Alertmanager routes.
- Editing receivers.
- Reloading Alertmanager configuration.
- Sending test notifications.
- Creating or deleting receivers.
- Modifying notification templates.
- Editing inhibition rules.
- Routing simulation for arbitrary alerts.
- Tracking notification delivery history.
- Counting actual delivered notifications.
- Reading configuration files over SSH.
- Exposing raw secret-bearing configuration.
- A generic YAML editor.
- A new fuzzy-search algorithm.

---

## Acceptance Criteria

The milestone is complete when:

1. PromTUI has a dedicated Routing tab.
2. Routing contains separate Routes and Receivers views.
3. Users can browse the complete hierarchical routing tree.
4. Selected routes show matchers, receiver, grouping, timing and inheritance.
5. Users can navigate from a route to its effective receiver.
6. Users can browse all configured receivers, including unused ones.
7. Receiver details show integration types and safe destination information.
8. Users can see every route referencing a receiver, including inherited references.
9. Users can jump from a receiver reference to the corresponding route.
10. Search works in both views using existing fuzzy-search behavior.
11. Configuration is fetched from Alertmanager API v2.
12. Sensitive information is redacted.
13. Connection switching and refresh work correctly.
14. No Alertmanager configuration is modified.
15. Tests and documentation are complete.
