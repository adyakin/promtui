# PromTUI — Configurable Alerts and Targets Columns & Search

## Objective

Implement per-connection configuration of visible table columns and searchable fields for the **Alerts** and **Targets** tabs.

This is particularly useful for VictoriaMetrics environments using vmagent and Consul service discovery, where custom Prometheus labels such as `servicename` identify projects or services more meaningfully than default fields like `job` or `instance`.

The implementation must:

- Allow users to explicitly define which columns appear, in what order.
- Allow users to independently define which fields participate in fuzzy search.
- Support built-in fields and arbitrary Prometheus labels.
- Preserve current behavior when configuration is absent.
- Require no changes to existing keybindings, query execution, API requests, or fuzzy-search algorithms.
- Use YAML configuration only.

Before implementation, inspect the existing Alerts and Targets models, rendering, filtering, sorting, configuration handling, and all `docs/uix*.md` files.

## 1. Configuration Schema

Extend each connection with optional `alerts` and `targets_view` settings.

Example:

```yaml
connections:
  production:
    url: https://victoriametrics.example.com
    alertmanager_url: https://alertmanager.example.com

    alerts:
      columns:
        - state
        - field: labels.severity
          title: Severity
        - field: labels.servicename
          title: Service
        - field: labels.alertname
          title: Alert
        - duration

      search:
        - labels.servicename
        - labels.alertname
        - labels.instance

    vmagent:
      - name: moscow
        url: https://vmagent-msk.example.com
      - name: novosibirsk
        url: https://vmagent-nsk.example.com

    targets_view:
      columns:
        - health
        - field: labels.servicename
          title: Service
        - field: labels.instance
          title: Host
        - last_scrape

      search:
        - labels.servicename
        - labels.instance

  homelab:
    url: https://prometheus.example.com
```

### Important

The `vmagent` key defines scrape target sources.

Do not change its meaning or structure.

Use `targets_view` exclusively for Targets table presentation and search settings.

No new backend type or mode configuration is needed.

## 2. Replacement Semantics

Configured columns must **replace** the existing defaults, not extend them.

For example:

```yaml
alerts:
  columns:
    - state
    - labels.servicename
    - labels.alertname
```

The table must display exactly those three columns, in that order.

Do not automatically add `job`, `instance`, `severity`, or any other default column.

Search follows the same replacement rule.

```yaml
alerts:
  search:
    - labels.servicename
    - labels.alertname
```

Only those fields participate in fuzzy search.

### Independent defaults

Columns and search settings must be independently optional.

| Configuration | Columns | Search |
|---|---|---|
| Neither configured | Existing defaults | Existing defaults |
| Only `columns` configured | Custom | Existing defaults |
| Only `search` configured | Existing defaults | Custom |
| Both configured | Custom | Custom |

An explicitly empty `columns` or `search` list should be rejected with a clear configuration error, rather than silently falling back to defaults.

This avoids ambiguous behavior.

## 3. Field Reference Syntax

Use a consistent field-path syntax.

### Built-in fields

Built-in fields use their canonical names directly:

```yaml
columns:
  - state
  - duration
```

### Prometheus labels

Custom labels use the `labels.` prefix:

```yaml
columns:
  - labels.servicename
  - labels.alertname
  - labels.instance
```

Resolve `labels.servicename` as:

```go
record.Labels["servicename"]
```

Everything after `labels.` is treated as a literal label name.

Do not implement arbitrary JSONPath expressions or nested-object traversal.

### Supported fields

Support all existing built-in fields currently available in the Alerts and Targets tables.

Examples:

| View | Built-in fields |
|---|---|
| Alerts | `state`, `duration` |
| Targets | `health`, `last_scrape`, `scrape_url` |
| Both | Relevant existing source-specific fields |

Inspect the actual application models to determine the complete supported field list.

Do not invent unsupported fields or remove existing capabilities.

Custom `labels.*` fields must work without being predefined in the application.

### Out of scope

Do not support `discoveredLabels` or discovery metadata in this task.

Only final target labels from the API's `labels` map are required.

## 4. Column Definitions

Support two equivalent forms.

### Simple form

```yaml
columns:
  - state
  - labels.servicename
  - labels.alertname
```

### Expanded form

```yaml
columns:
  - state
  - field: labels.servicename
    title: Service
  - field: labels.alertname
    title: Alert
```

Both forms may appear in the same list.

### Column titles

If `title` is specified, display it exactly as the column heading.

Otherwise:

- Built-in fields use their existing display names.
- Label fields derive a readable title from the label name.

Examples:

| Field | Default heading |
|---|---|
| `state` | STATE |
| `health` | HEALTH |
| `labels.servicename` | SERVICENAME |
| `labels.alertname` | ALERTNAME |
| `last_scrape` | LAST SCRAPE |

Preserve the existing table heading style.

Do not introduce configurable column widths in this milestone.

## 5. Shared Field Resolution

Implement a reusable field-resolution mechanism for each view.

Conceptually:

```go
type Column struct {
    Field string
    Title string
}
```

The implementation should resolve a field reference into its corresponding value for a particular alert or target.

Examples:

```text
state               → alert.Status.State
labels.servicename  → alert.Labels["servicename"]

health              → target.Health
labels.servicename  → target.Labels["servicename"]
last_scrape         → target.LastScrape
```

The actual implementation must use the existing domain models.

### Missing values

If a configured label does not exist:

- Display `—` in the table.
- Treat the value as empty for search.
- Do not exclude the record.
- Do not fail rendering.
- Do not substitute another label automatically.

Missing values are normal in heterogeneous environments.

### Formatting

Preserve existing formatting for built-in fields:

- Durations
- Timestamps
- Health and alert states
- Existing special-value rendering

Do not convert all built-in values to raw strings without preserving their established display behavior.

## 6. Alerts Table

Apply configurable columns to the existing Alerts tab.

Requirements:

- Render only configured columns.
- Preserve configured order.
- Resolve custom fields from Alertmanager alert `labels`.
- Preserve existing state/severity filtering behavior.
- Preserve details overlay behavior.
- Preserve automatic refresh.
- Preserve selection across refresh where possible.
- Preserve existing error and empty states.

Custom columns must not alter which alerts are retrieved from Alertmanager.

### Example

```yaml
alerts:
  columns:
    - state
    - field: labels.servicename
      title: Service
    - field: labels.alertname
      title: Alert
    - duration
```

Expected layout:

```text
STATE     SERVICE       ALERT                DURATION
active    billing       HighCPU              2h 15m
active    payments      DiskSpaceLow         35m
silenced  auth          HighMemoryUsage      4h 10m
```

Do not display additional columns.

## 7. Targets Table

Apply configurable columns to the existing Targets tab.

Requirements:

- Render only configured columns.
- Preserve configured order.
- Resolve custom fields from final target `labels`.
- Support targets aggregated from multiple configured vmagent sources.
- Preserve source selection and source identity internally.
- Preserve health and active/dropped filters.
- Preserve automatic refresh.
- Preserve partial-source failure handling.
- Preserve target details overlay.
- Preserve selection across refresh where possible.

Do not require `source`, `job`, or `instance` to be visible columns.

These fields may remain necessary internally for identity, filtering, or source tracking, but they must not be forcibly displayed.

### Example

```yaml
targets_view:
  columns:
    - health
    - field: labels.servicename
      title: Service
    - field: labels.instance
      title: Host
    - last_scrape
```

Expected layout:

```text
HEALTH   SERVICE       HOST                LAST SCRAPE
up       billing       app01:9100          8s ago
up       payments      app02:9100          12s ago
down     auth          app03:9100          2m ago
```

## 8. Configurable Fuzzy Search

PromTUI already implements fuzzy search for Alerts and Targets.

**Reuse the existing implementation. Do not replace or redesign it.**

The current implementation searches a hardcoded collection of fields.

Replace that hardcoded field selection with values resolved from the configured `search` list.

Example:

```yaml
alerts:
  search:
    - labels.servicename
    - labels.alertname
    - labels.instance
```

For each alert:

1. Resolve the configured search fields.
2. Collect their values.
3. Pass those values to the existing fuzzy-search mechanism.
4. Preserve current matching, ranking, filtering and highlighting behavior where applicable.

### Requirements

- Search fields are independent of visible columns.
- Hidden fields may participate in search.
- Visible columns do not automatically become searchable.
- Missing labels contribute empty values.
- Preserve existing fuzzy-search behavior and keyboard shortcuts.
- Preserve search state during normal refreshes.
- Do not introduce a new search syntax.
- Do not add regex, expressions, or query-language features.

### Important

Do not concatenate fields in a way that introduces unintended cross-field matches if the current implementation matches individual fields independently.

Preserve existing matching semantics as closely as possible.

## 9. Sorting

Preserve existing sorting functionality.

Extend sorting to configurable columns where practical.

Requirements:

- Sorting must not depend on a fixed column position.
- Custom label columns should sort by their resolved string values.
- Missing values must be handled consistently.
- Built-in numeric and temporal fields must retain appropriate comparison semantics.
- Preserve the current sort direction and cycling behavior.
- Do not add a new sorting UI.

If existing sorting relies on fixed column indices, refactor it to use field identifiers.

Do not silently change existing default sorting behavior.

## 10. Table Layout

Reuse existing table rendering and layout mechanisms.

Requirements:

- Respect YAML column ordering.
- Preserve current focus and selection styling.
- Preserve existing active-panel borders.
- Handle narrow terminals gracefully.
- Truncate or adapt wide values using existing conventions.
- Do not introduce horizontal scrolling unless already supported.
- Avoid panics or invalid widths when many columns are configured.
- Preserve existing empty/loading/error views.

Follow all applicable `docs/uix*.md` specifications.

No new floating overlays are needed.

## 11. Configuration Validation

Validate configured fields when loading the configuration.

### Invalid built-in fields

Unknown built-in fields must produce an actionable error.

Example:

```text
Connection "production":
alerts.columns[2]: unknown field "unknown_field"
```

### Invalid label references

Reject malformed references such as:

```yaml
- labels.
```

### Invalid column objects

Reject:

- Missing `field`
- Empty field name
- Empty title when explicitly specified
- Unsupported object properties, if strict validation is used

### Duplicate columns

Reject duplicate field references within the same column list.

### Missing labels

Do not validate whether a particular custom label actually exists in the backend.

Label availability is dynamic and must not prevent application startup.

### Configuration isolation

Validate Alerts and Targets configuration independently.

Do not let one connection's custom fields leak into another connection.

## 12. Connection Switching

When switching connections:

- Apply the new connection's Alerts column configuration.
- Apply the new connection's Targets column configuration.
- Apply the corresponding search-field configuration.
- Fall back to defaults when settings are omitted.
- Rebuild visible rows using the new field definitions.
- Preserve existing connection-switching semantics.
- Do not reuse another connection's column definitions.

Ensure per-tab state remains isolated according to the existing application architecture.

## 13. Testing

### Configuration

- Default configuration preserves current behavior.
- Custom columns replace defaults.
- Custom search fields replace defaults.
- Columns and search can be configured independently.
- Column ordering is preserved.
- Simple and expanded column definitions work.
- Custom titles work.
- Invalid fields are rejected.
- Duplicate columns are rejected.
- Empty lists are rejected.
- Different connections use independent configurations.

### Field resolution

- Built-in alert fields resolve correctly.
- Built-in target fields resolve correctly.
- Arbitrary custom labels resolve correctly.
- Missing labels display `—`.
- Missing labels do not break search.
- Built-in formatting is preserved.

### Fuzzy search

- Existing fuzzy matching behavior remains unchanged.
- Configured fields are searchable.
- Unconfigured fields are not searchable.
- Hidden fields can be searched.
- Search works after connection switching.
- Search works after automatic refresh.

### Tables

- Alerts display exactly the configured columns.
- Targets display exactly the configured columns.
- Column order and titles are correct.
- Sorting works with custom fields.
- Narrow terminal rendering remains usable.
- Existing filters and detail overlays work.
- Multi-source Targets aggregation remains correct.

### Regression

- PromQL tab remains unchanged.
- Saved Queries remain unchanged.
- Alertmanager API integration remains unchanged.
- vmagent source configuration remains unchanged.
- OSC 7501 behavior remains unchanged.
- Existing keyboard shortcuts remain unchanged.

## 14. Non-Goals

Do not implement:

- Runtime column configuration
- Interactive column selection
- Per-tab configuration dialogs
- Discovered labels
- JSONPath
- Calculated columns
- Column width configuration
- Conditional formatting
- New fuzzy-search algorithms
- Search expressions or regex
- Connection-specific backend behavior changes
- Consul SD configuration or relabeling changes
- New API requests

All customization is defined through YAML.

## 15. Documentation

Document:

- `alerts.columns`
- `alerts.search`
- `targets_view.columns`
- `targets_view.search`
- Built-in field references
- `labels.*` syntax
- Simple and expanded column definitions
- Replacement semantics
- Default behavior
- Validation errors
- Examples for Prometheus and VictoriaMetrics/vmagent

Include a complete example showing custom `servicename` labels.

Follow existing milestone documentation conventions.

## Acceptance Criteria

The task is complete when:

1. Alerts and Targets columns can be configured independently per connection.
2. Configured columns completely replace default columns.
3. Custom Prometheus labels can be displayed using `labels.<name>`.
4. Column ordering and custom titles work.
5. Existing fuzzy search uses configured search fields.
6. Search fields are independent of visible columns.
7. Connections without customization retain their existing behavior.
8. Existing sorting, filtering, refreshing and navigation continue working.
9. No changes are required to Prometheus, Alertmanager, vmagent or Consul.
10. Configuration validation, tests and documentation are complete.

