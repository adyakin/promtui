# PromTUI — Milestone 11: Rules Explorer

## Objective

Implement a read-only **Rules Explorer** in PromTUI for inspecting Prometheus and VictoriaMetrics vmalert rule groups.

The feature must support:

- Alerting rules and recording rules.
- Rule groups and their evaluation status.
- Firing, pending and inactive alerting rules.
- Individual active alert instances associated with alerting rules.
- Rule expressions, labels, annotations and evaluation metadata.
- Multiple independent vmalert instances.
- Navigation from a rule to the PromQL editor.
- Partial failures when one of several rule sources is unavailable.

The implementation must follow PromTUI's existing **endpoint-role-based architecture**.

Do not introduce separate application backends for Prometheus and VictoriaMetrics.

## 1. Existing Architecture — Important Context

PromTUI currently uses a shared internal API layer.

The application does not have separate `PrometheusBackend` and `VictoriaMetricsBackend` interfaces.

Existing functionality:

| Feature | Current architecture |
|---|---|
| PromQL queries | Shared `api.Client`, supporting Prometheus-compatible endpoints |
| Autocomplete | Same API client, using label and metadata endpoints |
| Targets | Shared `api.Client.Targets`, supporting Prometheus and vmagent |
| Alerts | Alertmanager API v2 client |
| Silences | Alertmanager API v2 client |
| Routing | Alertmanager API v2 client |

The primary connection `url` may point to Prometheus, VictoriaMetrics single-node, or a vmselect tenant path.

The API client already handles URL prefixes and avoids duplicating `/api/v1`.

VictoriaMetrics-specific partial query results are normalized by the existing query decoder.

**Preserve this architecture.**

The Rules Explorer should extend the existing shared API client and introduce a normalized rules data model.

Do not add backend vendor detection, vendor configuration flags, or separate UI implementations.

## 2. Application Tab

Add a dedicated top-level **Rules** tab.

Assuming the existing Routing milestone has been merged, use:

```text
0 PromQL | 1 Alerts | 2 Silences | 3 Targets | 4 Routing | 5 Rules
```

If the actual tab layout differs, inspect the current application and integrate accordingly.

Requirements:

- Independent Rules state, loading and errors.
- Preserve selection and scroll position when switching tabs.
- Respect existing global navigation.
- Maintain contextual footer hints.
- Preserve Kitty graphics cleanup and OSC 7501 behavior.
- Follow all `docs/uix*.md` specifications.

## 3. Configuration

Introduce an optional per-connection `vmalert` configuration.

Example:

```yaml
connections:
  production:
    url: https://victoriametrics.example.com
    alertmanager_url: https://alertmanager.example.com

    vmalert:
      - name: moscow
        url: https://vmalert-msk.example.com
      - name: novosibirsk
        url: https://vmalert-nsk.example.com

  homelab:
    url: https://prometheus.example.com
```

### Resolution rules

**When `vmalert` is configured:**

Fetch rules exclusively from the listed vmalert instances.

Do not additionally fetch rules from the primary metrics URL.

**When `vmalert` is omitted:**

Fetch rules from the connection's primary `url`.

This supports standalone Prometheus installations without additional configuration.

### Validation

- Each configured source requires a nonempty `name` and `url`.
- Source names must be unique within the connection.
- Reject malformed URLs.
- Reject duplicate source names.
- Reject explicitly configured empty source lists rather than silently falling back.
- Do not add a `backend`, `mode`, or `type` field.

### HTTP and TLS

Reuse existing connection-level HTTP/TLS configuration.

Respect:

- `insecure_skip_verify`
- Existing timeouts
- Existing authentication behavior, if supported
- Existing URL prefix handling

Do not create a separate HTTP transport implementation for vmalert.

## 4. Rules API

Use the Prometheus-compatible endpoint:

```http
GET /api/v1/rules
```

Prometheus and vmalert both expose rule information through this API family.

Support query parameters such as `type`, where useful and supported, but do not require server-side filtering for the initial implementation.

### Compatibility requirements

The agent must inspect the actual Prometheus and vmalert API response formats before implementing the decoder.

Use official API documentation or representative fixtures for both.

Do not assume every field is identical.

In particular, inspect differences involving:

- Rule group metadata
- Alerting rule state
- Active alert instances
- Rule health
- Evaluation errors
- Evaluation timestamps
- Evaluation duration
- Rule group identifiers
- Recording rule metadata

Implement tolerant decoding of optional and backend-specific fields.

Do not discard useful supported fields merely because they are absent from one backend.

### Important architecture rule

Backend-specific response handling belongs in the API decoding/normalization layer.

The Rules UI must consume a common internal representation.

Avoid code such as:

```go
if backend == "victoriametrics" {
    // render vmalert rules
} else {
    // render Prometheus rules
}
```

Instead, normalize supported fields and render based on the normalized data.

If genuinely different API paths or query parameters are required, isolate those differences inside a small API-layer compatibility mechanism.

Do not introduce a whole-stack backend adapter.

## 5. Internal Data Model

Create a normalized representation of rule sources, groups, rules and alert instances.

Conceptually:

```go
type RuleSource struct {
    Name   string
    URL    string
    Groups []RuleGroup
}

type RuleGroup struct {
    Name     string
    File     string
    Interval string
    Rules    []Rule
}

type Rule struct {
    Name        string
    Type        RuleType
    Expression  string
    Health      string
    LastError   string
    Labels      map[string]string
    Annotations map[string]string
}
```

Extend these conceptual types with the metadata required below.

The actual model should include:

### Rule group

- Source identity
- Group name
- Group file or identifier, if provided
- Evaluation interval
- Last evaluation
- Evaluation duration
- Evaluation limit, if provided
- Ordered list of rules

### Alerting rule

- Rule name
- Expression
- State: firing, pending or inactive
- Health
- Last error
- `for` duration
- `keep_firing_for`, if available
- Labels
- Annotations
- Active alert instances
- Evaluation metadata, if provided

### Recording rule

- Recording rule name
- Expression
- Labels
- Health
- Last error
- Evaluation metadata, if provided

### Alert instance

- State
- Labels
- Annotations
- Active/pending start timestamp, where provided
- Current value, where provided
- Other useful supported metadata

Use optional fields when data is not available.

Never invent missing timestamps, durations or states.

### Stable identity

Rule names are not globally unique.

Identify rules using source identity, group identity and position within the group.

Preserve selection across refresh where possible.

Do not use rule name alone as a unique key.

## 6. Rules Explorer Layout

Use a two-panel layout.

Left panel: hierarchical rule browser.

Right panel: selected group, rule or alert-instance details.

Conceptual layout:

```text
┌─ Rules ──────────────────────────────────┬─ Rule details ───────────────────────────┐
│                                         │                                         │
│ ▼ moscow                                │ HostDiskWillFill                        │
│   ▼ node-exporter                       │ Type: Alerting                          │
│     ▼ HostDiskWillFill       FIRING     │ State: Firing                           │
│         server01             FIRING     │ Health: OK                              │
│         server02             PENDING    │ For: 10m                                │
│       HostHighCPU            INACTIVE   │                                         │
│   ▼ recording-rules                     │ Expression:                             │
│       instance:cpu:rate5m    RECORD     │ predict_linear(...) < 0                │
│                                         │                                         │
│ ▼ novosibirsk                           │ Active alerts: 2                        │
│   ▼ infrastructure                      │                                         │
│       HostMemoryPressure     FIRING     │ p → Load expression into PromQL         │
└─────────────────────────────────────────┴─────────────────────────────────────────┘
```

For a single source, avoid unnecessary visual clutter from an extra source hierarchy level.

For multiple sources, preserve clear source identification.

### Tree hierarchy

Use:

```text
Source
  Group
    Rule
      Alert instance
```

Alert instances are children of alerting rules.

Recording rules do not have alert-instance children.

### Navigation

Suggested keys:

| Key | Action |
|---|---|
| `j` / `↓` | Next visible item |
| `k` / `↑` | Previous visible item |
| `h` / `←` | Collapse / move toward parent |
| `l` / `→` | Expand |
| `Enter` | Expand or open details |
| `p` | Load selected rule expression into PromQL |
| `/` | Fuzzy search |
| `f` | Filter by rule type |
| `s` | Filter by rule state |
| `r` | Refresh |
| `Esc` | Back / close overlay |

Adapt shortcuts to existing application conventions and avoid conflicts.

## 7. Rule Type and State Filtering

Show both alerting and recording rules by default.

Provide a type filter:

```text
All | Alerting | Recording
```

For alerting rules, provide a state filter:

```text
All | Firing | Pending | Inactive
```

### Important

Alerting rule state is not Alertmanager notification state.

For example:

- A firing rule may produce an alert silenced by Alertmanager.
- A pending alert may not yet have reached Alertmanager.
- An inactive rule may have no active alert instances.

Do not attempt to derive Alertmanager silence or inhibition state from rule evaluation data.

### Filtering hierarchy

When a filter matches a rule, keep its parent source and group visible.

Do not show empty groups unless the selected view explicitly calls for them.

Preserve original ordering within groups.

## 8. Rule Details

### Alerting rules

Display:

- Name
- Source
- Group
- Expression
- State
- Health
- Last error
- `for`
- `keep_firing_for`, if available
- Labels
- Annotations
- Last evaluation
- Evaluation duration
- Active alert instance count
- Pending alert instance count

### Recording rules

Display:

- Name
- Source
- Group
- Expression
- Health
- Last error
- Labels
- Last evaluation
- Evaluation duration

### Evaluation errors

Show concise errors inline.

Provide a scrollable details overlay for long error messages, following existing PromTUI error-display conventions.

### Expressions

Preserve complete multiline expressions.

Wrap long expressions without losing content.

Do not truncate the underlying expression stored in the model.

## 9. Alert Instance Details

Alerting rules may contain multiple active alert instances.

For example:

```text
HostDiskWillFill

  server01   FIRING
  server02   PENDING
  server03   FIRING
```

Allow selecting an instance and inspecting:

- State
- Labels
- Annotations
- Current value
- Active/pending start time, if available
- Parent rule
- Source

Instances should remain associated with their parent rule.

Do not merge instances from identically named rules across different groups or sources.

Do not assume an alert instance exists in Alertmanager.

## 10. Rule → PromQL Navigation

Pressing `p` on a selected rule must:

1. Obtain the complete rule expression from the normalized model.
2. Switch to the PromQL tab.
3. Focus the PromQL editor.
4. Replace the editor's current expression.
5. Preserve multiline formatting.
6. **Not automatically execute the query.**

Preserve:

- Current connection
- Query mode
- Time range
- Query history
- Saved Queries
- Existing query results until explicitly executing another query

This must reuse the existing editor-navigation mechanism introduced for Alert Details → PromQL.

Do not duplicate editor state mutation logic.

### Alert instances

When an alert instance is selected, `p` should load the parent rule's expression.

Do not automatically append instance label matchers or rewrite the expression.

### Recording rules

Load the recording rule's defining expression, not merely the recording metric name.

### MetricsQL compatibility

Some vmalert rules may use MetricsQL-specific syntax.

The editor should load the expression unchanged.

Existing local Prometheus-parser diagnostics may flag unsupported MetricsQL syntax.

Do not reject or modify the expression because of those diagnostics.

Query execution must remain independent of local diagnostic warnings.

Improving MetricsQL diagnostics is outside this milestone.

## 11. Multi-Source Fetching

When multiple `vmalert` sources are configured:

- Fetch concurrently.
- Use independent requests and error handling.
- Preserve each source's identity.
- Display successful results even if another source fails.
- Support manual refresh.
- Prevent stale responses from overwriting newer results.
- Cancel obsolete requests when appropriate.

### Partial failure example

```text
Rules

moscow        OK       12 groups
novosibirsk   ERROR    Connection timeout

Showing rules from 1 of 2 sources
```

A failed source must not cause healthy sources to disappear.

### Refresh behavior

Use existing refresh conventions.

A refresh interval around 30 seconds is reasonable, but prefer consistency with the application's current polling architecture.

If a previously successful source fails during refresh, preserve its last valid data and visibly mark it stale.

Do not silently present stale rules as current.

## 12. Search

Reuse the existing fuzzy-search implementation.

Search across:

- Rule name
- Group name
- Source name
- Expression
- Rule labels
- Rule annotations

For alert instances, include instance labels.

### Requirements

- Search must work across collapsed groups.
- Matching items should reveal their ancestors.
- Preserve fuzzy ranking and highlighting conventions.
- Avoid rebuilding large search indexes unnecessarily.
- Handle no-match states.
- Keep filtering and searching compatible.

Do not implement a separate fuzzy matcher.

## 13. Group Evaluation Metadata

Where supported, display:

- Evaluation interval
- Last evaluation timestamp
- Evaluation duration
- Evaluation limit
- Group identifier/file

Do not assume these fields are identical between Prometheus and vmalert.

For unavailable fields, show `—` or omit them consistently.

Do not substitute guessed values.

## 14. Prometheus and vmalert Compatibility

This section is particularly important.

### Prometheus

The fallback source is the connection's primary `url`.

Use the existing API client to fetch:

```http
GET /api/v1/rules
```

Preserve URL path prefixes and existing authentication/TLS behavior.

### VictoriaMetrics / vmalert

When `vmalert` sources are configured, fetch rules directly from those instances.

Do not query the central VictoriaMetrics metrics endpoint for rule definitions.

vmalert exposes Prometheus-compatible rule APIs, but the agent must verify its supported fields and response structure.

### Compatibility approach

Prefer this sequence:

1. Extend the existing API client with a `Rules` operation.
2. Decode the common Prometheus-compatible response.
3. Normalize supported rule/group/alert fields.
4. Add narrowly scoped compatibility handling only for verified differences.
5. Keep the UI backend-agnostic.

If vmalert requires a different path or parameter for a particular capability, implement that variation within the API layer.

Do not scatter URL checks, vendor checks or response-shape conditionals throughout Bubble Tea components.

### Avoid unnecessary abstractions

Do not introduce:

- `PrometheusBackend`
- `VictoriaMetricsBackend`
- Backend factory
- Vendor registry
- Backend mode selector
- Vendor-specific Rules tabs

The existing endpoint-role architecture is sufficient.

## 15. Error Handling

Handle:

- Missing or malformed source URL
- Connection timeout
- HTTP errors
- Invalid JSON
- API-level errors
- Missing rule groups
- Unknown rule types
- Unknown rule states
- Missing optional fields
- Partially malformed response data
- Failed refresh after a successful load
- Connection switching during an active request

Errors must be associated with the source that produced them.

Do not expose authentication credentials or sensitive URL components in UI errors or logs.

## 16. Testing

### API compatibility

Create representative fixtures for:

- Prometheus alerting rules
- Prometheus recording rules
- vmalert alerting rules
- vmalert recording rules
- Mixed groups
- Active alert instances
- Empty rule groups
- Missing optional fields
- Evaluation errors

Use actual documented response shapes rather than inventing a shared JSON schema and assuming both backends produce it.

### Configuration

Test:

- No `vmalert` key → primary URL fallback
- One vmalert source
- Multiple vmalert sources
- Explicit empty list
- Duplicate source names
- Invalid source URLs
- TLS inheritance

### Normalization

Test:

- Alerting vs recording rules
- Firing, pending and inactive states
- Rule health
- Evaluation errors
- Rule labels and annotations
- Group metadata
- Active alert instances
- Missing fields
- Stable rule identity

### UI

Test:

- Tree expansion/collapse
- Source/group/rule/instance navigation
- Type filter
- State filter
- Fuzzy search
- Search in collapsed groups
- Rule details
- Instance details
- Rule → PromQL
- Multiline expression preservation
- No automatic query execution
- Terminal resizing

### Multi-source

Test:

- All sources successful
- One source failed
- All sources failed
- Partial refresh failure
- Stale data indication
- Out-of-order responses
- Connection switching during fetch

### Regression

Verify:

- Existing query execution
- Autocomplete
- Alertmanager alerts
- Silences
- Routing
- Targets
- Saved Queries
- Generator URL → PromQL
- Kitty graphics
- OSC 7501
- Global keyboard shortcuts

## 17. Documentation

Update the relevant documentation with:

- Rules tab usage
- Keyboard shortcuts
- Rule hierarchy
- Alerting and recording rules
- Alert instance inspection
- Type and state filters
- Rule → PromQL navigation
- `vmalert` configuration
- Primary URL fallback
- Multiple vmalert sources
- Partial failure behavior
- Prometheus/vmalert API compatibility
- Known MetricsQL diagnostic limitations

## 18. Non-Goals

Do not implement:

- Rule editing
- Rule creation/deletion
- Reloading Prometheus or vmalert configuration
- Editing evaluation groups
- Alertmanager silence creation from Rules
- Linking rule instances directly to Alertmanager alerts
- Replacing the existing generator URL integration
- Query expression rewriting based on alert labels
- MetricsQL parser support
- Backend vendor detection
- New backend adapter architecture
- Rule evaluation simulation
- Notification delivery tracking

## Acceptance Criteria

The milestone is complete when:

1. A dedicated Rules tab exists.
2. Alerting and recording rules are displayed together by default.
3. Users can browse rule sources, groups, rules and active alert instances.
4. Rules show their expressions, state, health and evaluation metadata.
5. Rule details expose labels, annotations, durations and errors.
6. Users can filter by rule type and alerting state.
7. Existing fuzzy search works across rules and groups.
8. Pressing `p` loads a rule expression into PromQL without executing it.
9. Standalone Prometheus works without additional configuration.
10. Multiple vmalert sources work through the `vmalert` YAML key.
11. Partial source failures do not hide healthy sources.
12. Prometheus/vmalert differences are handled in the API layer.
13. No vendor-specific application backend abstraction is introduced.
14. Existing functionality remains intact.
15. Tests and documentation are updated.
