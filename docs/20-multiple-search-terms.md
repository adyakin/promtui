# PromTUI — Shared AND Search for Alerts and Targets

## Goal

Extend the existing fuzzy search functionality in PromTUI to support multiple AND conditions and field-specific matching in both the **Alerts** and **Targets** tabs.

Improve the Alerts header so saved searches are clearly identified.

Keep the implementation lightweight, reuse existing search infrastructure, and preserve backward compatibility.

## 1. Shared search engine

Implement a reusable search parser and evaluator used by both Alerts and Targets.

The shared component should handle:

- Multiple search terms.
- Logical AND between terms.
- Case-insensitive matching.
- Existing fuzzy/substring matching behavior.
- Optional field qualifiers.
- Quoted values containing spaces.

Each tab provides access to its own searchable object fields.

Do not implement separate search parsers for Alerts and Targets.

### AND semantics

Multiple space-separated terms must all match the same object.

Examples:

```text
HaproxyHasNoAliveBackends okd4-sov-test
NodeDown critical
postgresql production
```

Each term may match a different field.

For example, an alert with:

```yaml
labels:
  alertname: HaproxyHasNoAliveBackends
  servicename: okd4-sov-test
  severity: critical
```

must match:

```text
HaproxyHasNoAliveBackends okd4-sov-test
```

but must not match:

```text
HaproxyHasNoAliveBackends staging
```

Unqualified terms search across the object's existing searchable fields, independently of which columns are visible.

Search operates on already-loaded data. Do not make additional API requests.

## 2. Field-specific search

Support the following syntax:

```text
alertname:HaproxyHasNoAliveBackends servicename:okd4-sov-test
severity:critical instance:192.168
job:node-exporter health:down
backend:postgresql
```

Field-qualified terms search only the specified field.

### Arbitrary labels

**Do not maintain a whitelist or registry of available labels.**

Alerts and Targets already contain dynamic label maps. Resolve field names directly against those maps.

For example:

```text
servicename:payments
datacenter:msk
custom_label:whatever
```

If the label exists, search its value.

If it doesn't exist, the term simply doesn't match.

No error, warning, schema validation, or label discovery is required.

### Explicit namespaces

Also support:

```text
labels.servicename:payments
labels.datacenter:msk
annotations.summary:"connection refused"
```

Use `labels.*` for explicit label lookup and `annotations.*` for Alertmanager alert annotations.

For Targets, annotation lookups naturally return no match.

### Built-in fields

Support existing object fields that aren't necessarily present in label maps.

Examples:

**Alerts**

```text
alertname:NodeDown
severity:critical
instance:192.168
```

**Targets**

```text
health:down
scrapeUrl:metrics
job:node-exporter
instance:192.168
```

Resolution rules:

1. Explicit `labels.*` and `annotations.*` prefixes select those maps.
2. For unprefixed field names, resolve relevant built-in object fields first.
3. Otherwise look up the field in the object's label map.
4. Missing fields return no match.

Use case-insensitive matching for values. Preserve the existing conventions for label-key lookup where possible.

Avoid introducing configuration for searchable field names.

## 3. Quoted search terms

Support quoted values containing spaces.

Examples:

```text
"connection refused" servicename:payments
annotations.summary:"backend unavailable"
job:node-exporter "connection timeout"
```

Quoted strings count as one term.

Keep the parser simple.

Do not implement:

- OR
- NOT
- Parentheses
- Regex operators
- PromQL-style expressions
- Complex escaping rules beyond what is necessary for quoted terms

Malformed input should not crash the UI. Handle incomplete quotes gracefully during interactive typing.

## 4. Alerts integration

Extend the existing `/` search in the Alerts tab.

Requirements:

- Preserve existing single-term behavior.
- Apply AND semantics for multiple terms.
- Support arbitrary alert labels and annotations.
- Keep searching independent of the selected table view.
- Preserve existing sorting and saved filters.
- Preserve existing label-matcher filtering.
- Combine active saved filters and fuzzy search using the current filtering pipeline.

Do not change the format of existing saved searches.

## 5. Targets integration

Extend the existing Targets search using the same shared parser and evaluator.

Examples:

```text
node-exporter production
job:node-exporter instance:192.168
health:down servicename:postgresql
datacenter:msk job:vmagent
```

Requirements:

- Search all currently loaded targets from configured target sources.
- Support arbitrary labels.
- Support target-specific built-in fields such as `health` and `scrapeUrl`.
- Preserve existing target grouping, sorting, selection, and details.
- Do not introduce additional API requests.

Alerts and Targets should have independent search state.

Switching tabs must not overwrite the other tab's search expression.

## 6. Autocomplete

Preserve the existing Alerts search autocomplete and adapt it to multiple terms.

Autocomplete should operate on the term currently being edited, not on the entire search expression.

Example:

```text
HaproxyHasNoAliveBackends okd
```

Selecting `okd4-sov-test` should produce:

```text
HaproxyHasNoAliveBackends okd4-sov-test
```

For qualified terms:

```text
servicename:okd
```

Selecting `okd4-sov-test` should produce:

```text
servicename:okd4-sov-test
```

Requirements:

- Preserve previously entered terms.
- Preserve field prefixes.
- Continue supporting Enter without selecting a suggestion.
- Do not introduce API requests for autocomplete.
- Reuse existing autocomplete infrastructure.

For Targets, reuse autocomplete functionality where straightforward, but do not build a new label-discovery framework or complex suggestion system.

## 7. Alerts header improvement

Currently the Alerts top border displays the active saved search name without a label.

Example:

```text
Alerts | View: haproxy | OKD haproxy
```

Change it to:

```text
Alerts | View: haproxy | Search: OKD haproxy
```

The top-border `Search:` field represents the **name of the active saved search**, not the raw search expression.

The existing second-line `Filter:` field already displays the actual search/filter expression.

Examples:

**No saved search active**

```text
Alerts | View: default
Filter: Node · Sort: state/severity
```

**Saved search active**

```text
Alerts | View: haproxy | Search: OKD haproxy
Filter: HaproxyHasNoAliveBackends okd4-sov-test · Sort: state/severity
```

**Saved search modified**

If the user changes the expression so it no longer represents the selected saved search, clear the saved search name from the header.

Do not display stale saved-search metadata.

Preserve responsive header sizing and truncation.

### Targets header

Keep the existing Targets header unless it already supports named saved searches.

Do not add a new saved-search persistence system to Targets as part of this task.

## 8. Architecture

Prefer a small shared package or existing internal utility with an interface similar to:

```go
type SearchTerm struct {
    Field string
    Value string
}

func ParseSearch(query string) []SearchTerm

func MatchAll(
    terms []SearchTerm,
    resolve func(field string) []string,
    searchAll func() []string,
) bool
```

This is illustrative, not a required API.

The implementation should reuse existing matching behavior rather than replace it with an incompatible algorithm.

Avoid unnecessary abstractions, schemas, registries, or configuration.

Parse the search expression when it changes, not repeatedly for every object on every render.

## 9. Testing

Add focused unit tests for the shared parser and evaluator:

- Single-term backward compatibility.
- Two or more AND terms.
- Terms matching different fields.
- Terms matching the same field.
- Case-insensitive matching.
- Field-specific matching.
- Arbitrary label names.
- Missing labels returning no match.
- Explicit `labels.*` and `annotations.*` namespaces.
- Built-in fields.
- Quoted values.
- Empty expressions.
- Incomplete quotes.
- Mixed qualified and unqualified terms.

Add integration tests for:

- Alerts filtering.
- Targets filtering.
- Independent search state between tabs.
- Existing saved searches.
- Existing label-matcher filters.
- Autocomplete replacing only the active term.
- Autocomplete preserving field prefixes.
- Alerts header with and without a saved search.
- Clearing the saved-search name after editing.
- Narrow terminal rendering.

Run:

```bash
go test ./...
```

Run existing formatting, linting, and build checks.

## 10. Acceptance criteria

1. `foo bar` matches only objects containing both terms.
2. Terms may match different fields of the same object.
3. `servicename:foo health:down` works in Targets.
4. `alertname:foo servicename:bar` works in Alerts.
5. Arbitrary labels work without registration or configuration.
6. Missing fields simply return no results.
7. Quoted terms work.
8. Alerts autocomplete supports multi-term expressions.
9. Existing saved searches and filters remain compatible.
10. The Alerts top border shows `Search: <saved search name>`.
11. The existing `Filter:` line continues to display the actual expression.
12. Alerts and Targets maintain independent search state.
13. Searching requires no additional API requests.
14. Existing keyboard navigation and UI behavior remain intact.

## Out of scope

- OR/NOT operators and parentheses.
- Regular-expression query syntax.
- Saved-search storage migrations.
- New saved-search functionality for Targets.
- Label schema validation.
- New configuration files.
- Changes to table column layouts.
- Graph UI improvements.
- Alert → Rule navigation.

## Implementation guidance

Inspect the existing Alerts and Targets search implementations before editing.

Reuse the existing search matching, autocomplete, and saved-search infrastructure wherever possible.

Prefer a minimal shared implementation with small tab-specific field resolvers.

Keep changes focused and update relevant documentation.
