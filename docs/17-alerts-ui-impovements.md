# PromTUI — Alerts UX Improvements

## Objective

Implement the complete Alerts UX improvement package using multiple coordinated OpenCode agent sessions.

The orchestrator is responsible for planning, delegation, integration, testing, review, and final merge into the original development branch.

Features:

1. YAML-defined named Alerts table views.
2. External view definitions from `alert_views.d/*.yaml`.
3. Per-connection default views and session-local active views.
4. Content-aware, weighted table column sizing.
5. Unified `Ctrl+O` picker for saved Alerts filters and views.
6. Alertname autocomplete in existing fuzzy search.
7. Alertmanager inhibition explainability in Alert Details.

Do not introduce an interactive view editor.

Do not rewrite the existing Alerts architecture or unrelated UI components.

## 1. Orchestration requirements

The main OpenCode session acts as orchestrator.

Use the installed **official Rex skill** to create and manage independent coding-agent sessions.

- Keep the orchestrator in the original Rex pane.
- Create one dedicated Rex pane per concurrent coding agent.
- Each agent gets its own Git worktree and feature branch.
- Use descriptive pane titles where supported.
- Give each agent explicit ownership boundaries and acceptance criteria.
- Do not have multiple agents edit the same working tree.
- Use native OpenCode subagents for short investigations or reviews.
- Do not close agent panes before their work has been reviewed and integrated.
- Avoid unnecessary agents if a task is too small to justify a separate session.

The orchestrator must first inspect the repository, existing configuration structures, Alerts implementation, saved-filter support, UI conventions, and `docs/uix*.md`.

Before parallel work begins, establish shared interfaces for view definitions, column specifications, picker items, and search suggestions.

Do not assume the proposed schemas exactly match the current repository. Adapt them while preserving the behavior specified below.

Protect existing uncommitted changes. Never use destructive Git operations or force-push without explicit authorization.

## 2. Agent 1 — Named views and configuration loading

**Ownership:** Configuration types, validation, view registry, YAML loading and associated tests.

Implement global named Alerts views.

Example:

```yaml
alerts:
  views:
    default:
      columns:
        - source: state
          width: 10
        - source: labels.severity
          width: 12
        - source: labels.alertname
          weight: 3
        - source: labels.instance
          weight: 3
        - source: duration
          width: 10

    victoria:
      columns:
        - source: state
          width: 10
        - source: labels.severity
          width: 12
        - source: labels.servicename
          weight: 2
        - source: labels.alertname
          weight: 3
        - source: labels.instance
          weight: 3
        - source: duration
          width: 10

    certificates:
      columns:
        - source: labels.severity
          width: 12
        - source: labels.servicename
          weight: 2
        - source: labels.path
          title: Certificate
          weight: 4
        - source: annotations.summary
          title: Expires in
          extract: 'expires in (.+?)\.?$'
          weight: 2
```

Supported column properties:

- `source`: built-in field, `labels.*`, or `annotations.*`.
- `title`: optional display heading.
- `width`: optional preferred fixed width in terminal cells.
- `weight`: optional flexible sizing weight.
- `extract`: optional regular expression containing one capture group.

`width` and `weight` are mutually exclusive.

If neither is provided, automatic sizing applies.

When `extract` does not match, display an empty/missing-value placeholder, not the full original annotation.

### External view files

Automatically discover:

```text
~/.config/promtui/alert_views.d/*.yaml
```

Each file contains:

```yaml
views:
  postgresql:
    columns:
      - source: labels.severity
        width: 12
      - source: labels.servicename
        weight: 2
      - source: labels.instance
        weight: 3
      - source: annotations.description
        title: Details
        weight: 4
```

Loading rules:

1. Load `alerts.views` from the main configuration.
2. Read matching external files in alphabetical order.
3. Merge complete named view definitions into one registry.
4. Reject duplicate view names with an error identifying both sources.
5. Do not deep-merge columns or individual view properties.
6. Missing or empty directories are valid.
7. Invalid YAML, regex, column configuration, or view references produce actionable errors.
8. No recursive includes, file watchers, or generic configuration include framework.

Resolve the external directory relative to the active PromTUI configuration directory when appropriate, so custom configuration locations remain usable.

### Per-connection defaults

Support a connection-specific setting:

```yaml
connections:
  - name: prometheus
    alerts:
      default_view: default

  - name: victoria-prod
    alerts:
      default_view: victoria
```

Adapt this example to the repository's existing connection schema.

The named view registry is global, but each connection has its own default and session-local active view.

Changing connections restores that connection's previously selected view during the session.

Pressing uppercase `D` resets the current connection to its configured default.

A missing or removed view after configuration reload falls back gracefully.

### Backward compatibility

Existing `alerts.columns` must continue to work when no named views are configured.

Existing `alerts.search` behavior must remain unchanged.

Existing `alerts.yaml` saved label filters are separate and must not be modified by the view loader.

## 3. Agent 2 — Table layout and column rendering

**Ownership:** Alerts table sizing, field extraction, column rendering, resize handling, and tests.

Implement a content-aware column layout algorithm.

Requirements:

- Compact columns such as State, Severity and Duration.
- Flexible columns such as Alert, Service, Instance, Path and Description.
- Allocate remaining available terminal width to flexible columns.
- Respect optional `width` and `weight`.
- Support automatic sizing without explicit configuration.
- Measure terminal display-cell width, not byte length.
- Account for borders, separators, padding and scrollbar.
- Truncate safely using `…`.
- Avoid layout jitter during scrolling.
- Recalculate on terminal resize or view change.
- Do not allow one exceptionally long annotation to dominate sizing.
- Handle narrow terminals gracefully.

Prefer stable widths based on column metadata and bounded content sampling rather than recalculating widths for every visible row.

If minimum usable widths exceed available terminal space, provide a sensible fallback. Horizontal scrolling may be implemented if it fits the existing table architecture; otherwise prioritize graceful truncation.

### Field rendering

Support built-in fields and dynamic labels/annotations using the existing field-resolution conventions.

Support optional regex extraction from annotation or label text.

Missing values should render consistently.

Keep selection highlighting, sorting, scrolling, and row navigation working.

The initial implementation targets Alerts. Make the layout logic reusable where practical, but do not refactor Targets or Rules unnecessarily.

## 4. Agent 3 — Unified picker and view interaction

**Ownership:** Alerts view selection, unified picker UI, keyboard handling, active-view state, and tests.

Extend the existing `Ctrl+O` behavior to display both:

- Saved Alerts label filters.
- Configured Alerts table views.

Use one searchable floating picker following existing PromTUI overlay conventions.

Each entry must clearly indicate its type.

Example:

```text
Open — Alerts

Search filters and views...

FILTER  HAProxy production
FILTER  Certificate alerts
VIEW    Default
VIEW    Victoria
VIEW    Certificates
VIEW    PostgreSQL
```

Selecting a **Filter** applies its saved label matchers without changing the active view.

Selecting a **View** changes the table columns without changing saved filters, search text, sorting, or status filters.

Other requirements:

- `Ctrl+O` opens the unified picker.
- `Ctrl+S` retains its existing saved-filter behavior.
- Uppercase `D` restores the current connection's default view.
- Existing lowercase `d` behavior must not be overwritten.
- Show the active view and saved filter names in the Alerts header when appropriate.
- Preserve selected view when opening Alert Details and returning.
- Preserve active view independently for each connection.
- Picker supports fuzzy searching, keyboard navigation, and `Esc`.
- Follow existing floating-overlay behavior; do not reflow the main layout.

Do not implement saving or editing views through the UI.

Do not introduce combined filter/view presets.

## 5. Agent 4 — Alertname autocomplete

**Ownership:** Existing Alerts fuzzy-search UI, autocomplete behavior, and tests.

Enhance `/` search with alertname suggestions.

Example:

```text
Search: Cert

  CertCheck
  CertExpiry
  CertificateRenewalFailed
```

Requirements:

- Suggestions come from distinct `alertname` label values in currently loaded Alerts data.
- Use the existing fuzzy matching approach.
- Match case-insensitively.
- Do not require additional API calls.
- Limit visible suggestions and support scrolling when necessary.
- Keyboard navigation must be predictable.
- Selecting a suggestion fills the existing fuzzy-search text with that alert name.
- Do **not** create an exact `alertname="..."` matcher.
- Pressing Enter without explicitly selecting a suggestion preserves existing fuzzy-search behavior across configured searchable fields.
- Do not automatically select the first suggestion.
- Escape closes autocomplete according to existing search conventions.
- Empty input and no-match results must behave gracefully.

Autocomplete must coexist with existing saved filters, status filters, and view selection.

Do not introduce a separate alertname-filtering mode.

## 6. Agent 5 — Alertmanager inhibition explainability

**Ownership:** Inhibition-rule evaluation, runtime relationship mapping, Alert Details integration, and tests.

This task is independent of the saved-view work and may run concurrently after repository inspection.

### Data

Use the existing Alertmanager client:

- `/api/v2/status` for `config.original` and `inhibit_rules`.
- `/api/v2/alerts` for alert state and runtime inhibition references.

Reuse existing fetched data and refresh behavior.

### Evaluation

Support Alertmanager inhibition semantics:

- Source and target matchers.
- Equality and inequality matchers.
- Regular-expression matchers.
- `equal` label comparisons.
- Missing and empty labels.
- Multiple rules and multiple source alerts.
- Self-inhibition prevention.
- Overlapping source/target matcher conditions.

Use runtime inhibition information as the source of truth whenever available.

Local rule matching is explanatory and must not be presented as confirmed runtime inhibition without supporting runtime evidence.

### Alert Details

For an inhibited alert, show:

- Inhibited state.
- Available inhibiting source alerts.
- Matching inhibition rules.
- Relevant equal-label values.
- Unresolved runtime references where applicable.

For an alert acting as an inhibition source, show:

- Currently inhibited targets.
- Number of targets.
- Relevant rules where identifiable.

Support keyboard navigation between related alerts without losing the existing Alerts view context.

Handle unavailable configuration or unresolved relationships without breaking the Alerts tab.

### Performance and correctness

Do not recompute all inhibition relationships during every render.

Cache or precompute relationships when alert data or configuration changes.

Avoid unnecessary pairwise scans for large alert sets.

Include thorough tests for missing labels, regex matching, multiple sources, overlapping matchers, and runtime/local discrepancies.

## 7. Integration plan

The orchestrator should use this sequence:

### Phase A — Inspect and establish interfaces

Inspect existing code, configuration, docs, and UI conventions.

Define shared interfaces and assign ownership boundaries.

Identify overlapping files before spawning concurrent agents.

### Phase B — Parallel implementation

Launch independent Rex agent sessions.

Recommended initial parallel work:

- Agent 1: Views/configuration.
- Agent 2: Table sizing/rendering.
- Agent 4: Autocomplete.
- Agent 5: Inhibition explainability.

Agent 3 may begin UI work concurrently if the view interfaces are stable; otherwise wait until Agent 1 has established them.

### Phase C — Integrate

Collect completed branches and integrate them into the orchestrator's integration branch.

Resolve conflicts and adapt interfaces as necessary.

Verify that view switching, table sizing, and the picker operate together.

### Phase D — Independent review

Use an independent review agent or native OpenCode subagent to inspect the integrated implementation.

Focus on:

- Backward compatibility.
- Configuration parsing and errors.
- Keyboard shortcut conflicts.
- Terminal resizing.
- Fuzzy-search behavior.
- Alertmanager inhibition correctness.
- Connection switching.
- Large alert sets.
- SSH terminal compatibility.

Address review findings before completion.

### Phase E — Verification and final merge

Run:

```bash
go test ./...
```

Also run existing lint, formatting, and build checks defined by the repository.

Update relevant documentation, example configuration, and keyboard shortcut references.

Perform interactive TUI verification where feasible.

Merge the completed implementation into the original development branch, preserving any existing user changes.

Do not stop after agents report success. The orchestrator owns the final integrated result.

## 8. Acceptance criteria

The task is complete when:

1. Views can be defined in `config.yaml` and external `alert_views.d/*.yaml` files.
2. View definitions merge deterministically and duplicate names produce useful errors.
3. Each connection can have its own default view.
4. `D` restores that connection's default view.
5. `Ctrl+O` shows both saved filters and configured views.
6. Applying a filter does not change the view, and applying a view does not change the filter.
7. `Ctrl+S` continues saving Alerts filters.
8. Labels and annotations work as table column sources.
9. Regex extraction works for values embedded in annotations.
10. Table widths use available terminal space intelligently.
11. `/` provides alertname autocomplete without changing existing fuzzy-search semantics.
12. Alert Details explain available inhibition relationships without confusing local inference with confirmed runtime inhibition.
13. Existing configurations remain compatible.
14. Tests, builds, documentation, integration and final merge are complete.

## 9. Out of scope

- Interactive view editor.
- Persisting active views between application launches.
- Combined filter/view presets.
- Automatic view selection based on alert type.
- Generic YAML includes.
- File watchers.
- New Alertmanager configuration-editing capabilities.
- Reworking unrelated tabs.
- Reimplementing the existing saved-filter parameter engine.
