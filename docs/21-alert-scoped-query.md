# PromTUI — Alert to Scoped Troubleshooting Graph

## Goal

Implement one-keypress navigation from an alert to a useful historical graph of the underlying metrics, scoped to the specific alert instance.

The primary goal is troubleshooting metric behavior before and during an alert, not displaying historical alert states through `ALERTS{}`.

Support both Prometheus and VictoriaMetrics/vmalert where possible.

## 1. Navigation actions

Add or update the following actions in Alert Details:

| Key | Action |
|---|---|
| `p` | Open scoped troubleshooting expression in PromQL graph mode |
| `P` | Open the exact original alert expression in PromQL |
| `r` | Navigate to the exact generating rule in Rules |

Update contextual footer hints.

Reuse existing PromQL and Rules tabs. Do not introduce a separate troubleshooting screen.

Preserve the selected alert and Alerts tab state when navigating away.

Check for existing shortcut conflicts before assigning keys.

## 2. Resolve original alert expression

### Prometheus

Reuse the existing generatorURL expression extraction mechanism.

Preserve existing Prometheus compatibility.

### VictoriaMetrics/vmalert

vmalert generator URLs contain alert and group identifiers instead of the original expression.

Real example from the production environment:

```text
https://S-NSK90-APP0231:9431/vmalert/alert?group_id=2485214638202814937&alert_id=4564276932026302966
```

Parse `group_id` and `alert_id` from the URL.

Use the corresponding vmalert API endpoint:

```text
GET /vmalert/api/v1/alert?group_id=2485214638202814937&alert_id=4564276932026302966
```

This endpoint has been manually verified against the actual production vmalert instance.

Relevant fields from the real response:

```json
{
  "state": "firing",
  "name": "HaproxyHasNoAliveBackends",
  "value": "0",
  "labels": {
    "alertgroup": "haproxy",
    "alertname": "HaproxyHasNoAliveBackends",
    "instance": "S-NSK90-MS00143.t-global.bcs:7000",
    "job": "consul-sd",
    "proxy": "minio",
    "servicename": "bcs-minio-clicard-01",
    "severity": "critical",
    "sourceservicename": "bcs-vic"
  },
  "activeAt": "2026-11-24T07:20:00+03:00",
  "id": "4564276932026302966",
  "rule_id": "3452656140727507818",
  "group_id": "2485214638202814937",
  "expression": "haproxy_backend_active_servers{proxy!=\"stats\"} + haproxy_backend_backup_servers == 0"
}
```

The labels above are a subset of the actual response.

Keep identifiers as strings to avoid precision loss.

### Source resolution

Resolve the generator URL against configured vmalert sources.

Requirements:

- Support multiple vmalert sources per connection.
- Respect configured TLS verification settings.
- Reuse existing HTTP client infrastructure.
- Preserve configured URL prefixes.
- Handle hostname case differences.
- Do not arbitrarily request unconfigured destinations supplied by generator URLs.
- Do not silently select the first vmalert source when source resolution is ambiguous.

Resolve metadata on demand when the user invokes a navigation action.

Do not request individual alert metadata during normal Alerts refresh.

Handle alerts that have already resolved and are no longer available through the individual alert endpoint.

Where possible, reuse cached rule information as a fallback, but never silently select an ambiguous rule.

## 3. Troubleshooting expression generation

The original alert expression is often unsuitable for historical metric inspection because it:

- Selects metrics for many targets.
- Contains a threshold comparison that filters out healthy historical values.

The troubleshooting expression should address both problems.

### Expression parser

Investigate and use the Go MetricsQL parser:

```text
github.com/VictoriaMetrics/metricsql
```

Use AST-based expression manipulation.

Do not implement PromQL/MetricsQL rewriting through regular expressions or string splitting.

Ensure generated expressions remain compatible with the selected backend. In particular, do not introduce MetricsQL-only syntax into Prometheus queries.

Keep the original expression unchanged and available through `P`.

## 4. Remove the alert threshold

For simple outer comparisons against scalar thresholds, remove the comparison and retain the underlying metric expression.

Examples:

**Original:**

```promql
up == 0
```

**Troubleshooting:**

```promql
up
```

**Original:**

```promql
rate(http_requests_total[5m]) > 100
```

**Troubleshooting:**

```promql
rate(http_requests_total[5m])
```

**Original:**

```promql
haproxy_backend_active_servers{proxy!="stats"}
+ haproxy_backend_backup_servers == 0
```

**Troubleshooting:**

```promql
haproxy_backend_active_servers{proxy!="stats"}
+ haproxy_backend_backup_servers
```

Also handle scalar comparisons where the scalar is on the left, when unambiguous.

Preserve:

- Arithmetic expressions.
- Functions.
- Range selectors.
- Aggregations.
- Existing label matchers.
- Vector matching modifiers.
- Subqueries.
- Offset modifiers.

Only remove a clearly identified outer alert condition.

Do not recursively remove comparisons inside functions, nested expressions, or logical operations.

For complex expressions where removing a comparison could change the meaning of the underlying calculation, preserve the original expression and indicate that threshold removal was skipped.

## 5. Scope the expression to the selected alert

The generated query should return metrics corresponding to the selected alert instance rather than all instances covered by the rule.

### Real HAProxy example

Original expression:

```promql
haproxy_backend_active_servers{proxy!="stats"}
+ haproxy_backend_backup_servers == 0
```

Selected alert labels include:

```yaml
instance: S-NSK90-MS00143.t-global.bcs:7000
servicename: bcs-minio-clicard-01
proxy: minio
severity: critical
alertname: HaproxyHasNoAliveBackends
```

Desired troubleshooting expression:

```promql
haproxy_backend_active_servers{
  proxy!="stats",
  proxy="minio",
  instance="S-NSK90-MS00143.t-global.bcs:7000",
  servicename="bcs-minio-clicard-01"
}
+
haproxy_backend_backup_servers{
  proxy="minio",
  instance="S-NSK90-MS00143.t-global.bcs:7000",
  servicename="bcs-minio-clicard-01"
}
```

This is the desired result when the relevant labels are present on both underlying metrics.

The exact generated formatting is not important.

### Label selection

Do not blindly inject all alert labels into metric selectors.

Alert labels can originate from:

- Underlying metric labels.
- Rule-defined labels.
- Group-defined labels.
- External labels.
- Aggregation output.
- Label manipulation functions.

Labels such as `alertname`, `severity`, and `alertgroup` frequently do not exist on the underlying metrics.

Prefer useful identifying labels such as:

- `instance`
- `servicename`
- `job`
- `proxy`
- `mountpoint`
- `device`
- `namespace`
- `pod`
- `container`

This is not an exhaustive whitelist. Custom metric labels must also be supported when their applicability can be established.

Do not assume that a label exists on every metric selector simply because it exists on the alert.

### Safe scoping strategy

Implement conservative AST-based scoping.

For straightforward expressions containing a single metric selector, inject applicable identifying label matchers.

For expressions containing multiple selectors, only inject matchers where doing so preserves the calculation and the labels are applicable.

Preserve existing matchers. Do not silently overwrite conflicting constraints.

For complex expressions, consider filtering the final expression result rather than modifying its individual metric selectors.

VictoriaMetrics supports the MetricsQL `label_match()` function, which may be useful for result-level filtering. Investigate its semantics and suitability before implementation.

For example, result-level filtering could conceptually produce:

```metricsql
label_match(
  label_match(
    label_match(
      haproxy_backend_active_servers{proxy!="stats"}
      + haproxy_backend_backup_servers,
      "instance",
      "^S-NSK90-MS00143\\.t-global\\.bcs:7000$"
    ),
    "servicename",
    "^bcs-minio-clicard-01$"
  ),
  "proxy",
  "^minio$"
)
```

This is an illustrative alternative, not a mandatory generated expression.

Escape label values correctly when constructing exact-match regular expressions.

Prefer exact equality semantics over accidental regex matching.

### Aggregated expressions

Consider an alert expression such as:

```promql
sum by (servicename) (
  rate(http_requests_total[5m])
) > 100
```

If the alert represents a service, scope by `servicename`.

Do not inject an arbitrary `instance` restriction that changes a service-level aggregate into a single-host calculation.

### Complex expressions

Pay particular attention to:

- `on()` and `ignoring()`.
- `group_left()` and `group_right()`.
- Aggregations with `by()` and `without()`.
- `label_replace()` and other label manipulation functions.
- Logical operators.
- Recording rules.
- Multiple metric selectors with different label sets.

The first implementation does not need to handle every possible expression.

Correctness is more important than aggressive rewriting.

When safe scoping cannot be established, preserve the expression or apply only the transformations that are known to be safe.

Never silently present a potentially incorrect calculation as an exact alert-instance graph.

## 6. Transformation feedback

Provide concise feedback describing the generated query.

Example:

```text
Alert query · Scoped by instance, servicename, proxy · Threshold removed
```

Partial transformation:

```text
Alert query · Threshold removed · Instance scoping unavailable
```

Fallback:

```text
Alert query · Original expression used
```

Keep feedback unobtrusive and use existing status/message infrastructure.

The user must be able to distinguish:

- Fully scoped troubleshooting query.
- Partially transformed query.
- Original expression fallback.

The generated query remains editable in the normal PromQL input.

Do not automatically save generated queries as named queries.

## 7. Historical graph time range

When invoking `p`:

- Switch to PromQL.
- Populate the generated expression.
- Select range-query graph mode.
- Automatically execute the query.
- Use the TSDB associated with the current connection.

Use the alert's start time to choose a useful historical range.

Prefer vmalert `activeAt` when available, otherwise use the Alertmanager start timestamp.

The graph should include a meaningful period before the alert became active.

Suggested initial behavior:

- Minimum range: last hour.
- Include approximately one hour before the alert start when practical.
- End at the current time.
- Apply a reasonable upper bound to avoid unexpectedly expensive queries for long-running alerts.
- Reuse existing range presets and query-step calculation.

Do not introduce new time-range configuration options in this task.

Preserve the ability to manually adjust the range after navigation.

## 8. Original expression action

Uppercase `P` opens the exact original alert expression.

No threshold removal.

No additional label matchers.

No expression rewriting.

This provides a reliable fallback and makes it easy to inspect the original rule behavior.

Reuse the same expression resolver as `p`.

## 9. Alert to exact rule navigation

Implement `r` using the resolved vmalert metadata:

```json
{
  "rule_id": "3452656140727507818",
  "group_id": "2485214638202814937"
}
```

Identify the rule using:

- Configured vmalert source.
- Group ID.
- Rule ID.

Inspect the existing Rules API responses to verify how these identifiers are exposed.

When an exact match exists:

- Switch to Rules.
- Select the corresponding rule.
- Bring it into the visible table area.

Do not identify rules solely by alertname.

Multiple rules may share the same alertname and expression while having different thresholds, labels, or evaluation settings.

If exact ID-based resolution is unavailable, use existing rule-instance information where possible.

If multiple candidates remain, show them through the existing Rules search/filter mechanism rather than arbitrarily choosing one.

Preserve the ability to return to the originating alert.

## 10. Asynchronous behavior

Metadata resolution and query execution must not block the TUI event loop.

Use existing asynchronous command patterns.

Provide loading feedback while resolving the expression.

Handle:

- HTTP errors.
- Timeouts.
- Invalid generator URLs.
- Unknown vmalert sources.
- Missing expressions.
- Parser errors.
- Resolved alerts no longer available through the API.
- Ambiguous rule matches.

Avoid applying stale asynchronous responses if the user has switched connections or initiated another navigation action.

Reuse resolved metadata for repeated actions on the same alert where appropriate.

## 11. Testing

Add unit tests covering:

### vmalert resolution

- Generator URL parsing.
- API endpoint construction.
- Configured URL prefixes.
- Multiple vmalert sources.
- TLS configuration.
- Large identifiers preserved as strings.
- Real response fixture from this task.
- Missing or invalid response fields.

### Expression transformation

- Single metric selector.
- Existing equality and inequality matchers.
- Scalar threshold comparisons.
- Scalar on the left.
- Arithmetic expressions.
- Functions and range selectors.
- Aggregations.
- Multiple metric selectors.
- Vector matching.
- Label manipulation.
- Conflicting matchers.
- Missing metric labels.
- Escaping label values.
- Unsupported expressions and safe fallback.

Include the real HAProxy expression as a regression test.

### Navigation

- `p` opens and executes the troubleshooting graph.
- `P` preserves the original expression.
- `r` resolves the correct rule and source.
- Duplicate alertnames do not cause incorrect rule selection.
- Historical range includes time before alert activation when practical.
- Existing Prometheus generator URL behavior remains functional.
- Alerts selection and search state are preserved.
- Errors are displayed without breaking the UI.

Run:

```bash
go test ./...
```

Run existing formatting, linting, and build checks.

Perform smoke tests using the existing demo environment and Rex where possible.

## 12. Out of scope

- Dedicated `ALERTS{}` state-history functionality.
- Full graph UI redesign.
- Graph zooming and panning.
- Threshold overlays on graphs.
- Alert activation markers.
- New query editors.
- Automatic query persistence.
- New configuration systems.
- Exhaustive support for every possible MetricsQL expression.

## Acceptance criteria

1. Pressing `p` on a vmalert-generated alert opens a historical troubleshooting graph.
2. The original expression is resolved through the vmalert API.
3. Simple outer alert thresholds are removed.
4. The query is scoped to the selected alert instance when safe.
5. Healthy historical metric values remain visible.
6. Complex expressions have conservative, clearly communicated fallbacks.
7. Pressing `P` opens the exact original expression.
8. Pressing `r` navigates to the exact generating rule when identifiers permit.
9. Existing Prometheus support remains functional.
10. No per-alert metadata requests occur during ordinary Alerts refresh.
11. The generated expression remains editable.
12. Tests and documentation are updated.

The primary success criterion is practical:

**From a firing alert, one keypress should produce a useful graph for troubleshooting that specific alert instance.**
