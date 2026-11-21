# PromTUI — Saved Filters and Shared Parameter Engine

## Orchestrated implementation task

### Role

You are the **orchestrating development agent** responsible for delivering this feature end to end.

You may delegate implementation, tests and reviews to subagents, and use Git worktrees when beneficial.

**You own the final integration and merge.** Do not stop after producing an architecture proposal or after completing an individual phase.

The expected result is a working, tested, documented implementation integrated into the project's main development branch.

## 1. Objective

Extend PromTUI's existing Saved Queries functionality to support:

1. A reusable saved-item and parameter engine.
2. Saved Alertmanager alert filters.
3. Parameterized saved PromQL queries.
4. Shared parameter-input UI.
5. Independent, simple YAML storage formats.
6. Full compatibility with existing Saved Queries.

The implementation must proceed in three logical phases:

- **Phase A:** Shared engine and reusable UI infrastructure.
- **Phase B:** Saved Alerts.
- **Phase C:** Parameterized Saved PromQL.

Complete all phases, including integration, tests, documentation and final merge.

## 2. Mandatory repository inspection

Before modifying code:

- Inspect the repository structure and Go module.
- Read relevant architecture documentation.
- Read all `docs/uix*.md` files.
- Inspect the existing Saved Queries implementation.
- Inspect the Alerts list, filters, fuzzy search and API models.
- Inspect Bubble Tea models, messages, update handlers and overlays.
- Inspect existing configuration loading and atomic file persistence.
- Identify the default integration branch.
- Check Git status, existing worktrees and uncommitted changes.
- Inspect current keyboard shortcuts and their contextual handling.

Do not assume the package layout described below already exists.

Preserve existing application conventions.

### Existing architecture

PromTUI is a Go TUI using Bubble Tea.

It has an endpoint-role-based API architecture, not separate Prometheus and VictoriaMetrics application backends.

Existing saved PromQL queries are stored in:

`~/.config/promtui/queries.yaml`

They currently contain names and expressions.

Existing Saved Queries supports:

- Save
- Browse/load
- Rename
- Update
- Delete
- Fuzzy search
- Atomic persistence

Preserve these capabilities and their existing shortcuts.

## 3. Architecture principles

### Separate storage, shared behavior

Keep two independent files:

`~/.config/promtui/queries.yaml`

`~/.config/promtui/alerts.yaml`

Do not combine them into a polymorphic file.

Do not introduce `type: promql` or `type: alerts` discriminators in either YAML schema.

Each file must be independently readable, writable and easy to generate using external tools such as Ansible/Jinja2.

### Share only genuinely reusable code

Shared functionality should include:

- Saved-item name validation and uniqueness rules.
- File persistence helpers where appropriate.
- Atomic writes.
- Parameter definitions.
- Parameter reference parsing.
- Parameter resolution.
- Parameter validation.
- Reusable parameter-input UI.
- Reusable saved-item UI behavior where practical.

Domain-specific functionality stays separate:

**PromQL**

- Expression storage.
- Expression loading.
- PromQL-specific handling.

**Alerts**

- Label matcher storage.
- Matcher validation.
- Matcher evaluation.
- Alerts list filtering.

Do not force both domains into an unnecessarily generic interface.

Avoid speculative abstractions, complex registries, reflection-heavy designs or a general-purpose template engine.

Prefer simple Go code.

## 4. Shared parameter model

A parameter is a named string value.

Example:

```yaml
parameters:
  - name: service
    default: okd4-prod
```

References use:

`${service}`

### Resolution semantics

Parameter substitution is **raw string substitution**.

The engine must not:

- Escape regex characters.
- Escape PromQL strings automatically.
- Infer whether a parameter is literal or regex.
- Transform parameter values.
- Interpret parameter contents as another template.
- Expand environment variables.
- Execute functions.
- Perform recursive expansion.

The caller controls the meaning of the resolved string.

### Example

Template:

`.*${service}.*`

Input:

`service = okd4.prod`

Result:

`.*okd4.prod.*`

The dot remains a regex metacharacter.

This behavior is intentional.

### Required behavior

- Multiple references to one parameter resolve consistently.
- References may occur more than once.
- References may appear inside longer strings.
- Parameter defaults are optional.
- An explicitly supplied empty string is distinct from a missing value.
- Missing required values must be requested.
- Unknown references must produce validation errors.
- Duplicate parameter names must be rejected.
- Invalid parameter names must be rejected.
- Unused declared parameters should be reported as validation errors.
- Invalid placeholder syntax must not be silently ignored.
- Substitution must be single-pass, not recursive.
- Values containing `${other}` remain literal after insertion.
- Support literal dollar signs without accidentally treating them as parameters; document a simple escaping convention.

Choose a clear parameter-name syntax, such as:

`[A-Za-z_][A-Za-z0-9_]*`

Use the same syntax for both domains.

### Default values

A default value is an initial suggestion, not an immutable value.

The user may override it when loading a saved item.

A parameter without a default is required.

Preserve the distinction between:

- Default absent.
- Default present but empty.
- User-supplied empty string.

### Validation

Separate validation into two stages.

**Template validation**

Validate parameter declarations and references without knowing their runtime values.

**Resolved-content validation**

Validate the resulting content using the consuming domain's rules.

For Alerts, validate the final matcher and compile regex operators.

For PromQL, preserve the resolved expression unchanged. Do not reject it solely because the existing local Prometheus parser reports diagnostics, since MetricsQL-specific syntax may be valid on the selected backend.

## 5. Parameter engine API

Design a small, testable API independent of Bubble Tea.

Conceptually:

```go
type Parameter struct {
    Name    string
    Default *string
}

type Values map[string]string
```

Possible operations:

```go
ValidateDefinitions(...)
ExtractReferences(...)
Resolve(...)
MissingParameters(...)
```

These are conceptual operations, not mandatory names.

The engine must not depend on:

- Prometheus API clients.
- Alertmanager API clients.
- UI components.
- Current connection.
- Network access.

### Future autocomplete

We expect parameter value suggestions from the currently selected connection in a future milestone.

For example, a `service` parameter associated with the `servicename` alert label may eventually suggest known label values.

**Do not implement autocomplete now.**

However:

- Keep parameter definitions separate from input state.
- Keep the parameter dialog reusable.
- Avoid embedding network access inside the parameter engine.
- Avoid designing the dialog so that only static default values can ever be used.
- Allow a future asynchronous suggestion provider to be introduced without rewriting the entire dialog.

Do not create a speculative autocomplete framework or unused provider implementations.

## 6. Phase A — Shared engine

Implement and test the shared foundation first.

### Scope

- Parameter model.
- Template parsing.
- Reference extraction.
- Validation.
- Raw substitution.
- Missing-value detection.
- Shared parameter-input dialog.
- Reusable saved-item helpers where appropriate.
- Safe, incremental refactoring of existing Saved Queries.

### Compatibility

Existing `queries.yaml` files must continue working unchanged.

Do not require migration for existing users.

Do not modify existing Saved Queries behavior before the new implementation is ready.

### Parameter dialog

Build a reusable true floating overlay.

Example:

```text
┌─ Parameters: HAProxy by service ─────────┐
│                                         │
│ Service                                 │
│ [okd4-prod___________________________]  │
│                                         │
│ Tab: Next field                         │
│ Ctrl+Enter: Apply                       │
│ Esc: Cancel                             │
└─────────────────────────────────────────┘
```

Requirements:

- One input per declared parameter.
- Prefill defaults.
- Support multiple parameters.
- Keyboard navigation.
- Clear validation errors.
- Preserve entered values while navigating.
- `Ctrl+Enter` confirms.
- `Esc` cancels.
- Cancel must not mutate the consuming view or editor.
- Use existing overlay and focus conventions.
- No terminal layout reflow.

For parameterized items, show the dialog before applying the item.

For non-parameterized items, load immediately.

### Phase A completion gate

Before proceeding:

- Shared engine tests pass.
- Existing Saved Queries tests pass.
- Existing Saved Queries behavior remains unchanged.
- Shared dialog is integrated sufficiently to be reused.
- Architecture is reviewed for unnecessary abstraction.

Commit this phase separately.

## 7. Phase B — Saved Alerts

### Storage

Create:

`~/.config/promtui/alerts.yaml`

Example:

```yaml
alerts:
  - name: All HAProxy alerts
    matchers:
      - label: alertname
        operator: "=~"
        value: ".*HAProxy.*"

  - name: HAProxy — OKD4 Production
    matchers:
      - label: alertname
        operator: "=~"
        value: ".*HAProxy.*"
      - label: servicename
        operator: "="
        value: okd4-prod

  - name: HAProxy by service
    parameters:
      - name: service
        default: okd4-prod
    matchers:
      - label: alertname
        operator: "=~"
        value: ".*HAProxy.*"
      - label: servicename
        operator: "="
        value: "${service}"

  - name: HAProxy by service regex
    parameters:
      - name: service_pattern
        default: "okd4-.*"
    matchers:
      - label: servicename
        operator: "=~"
        value: "${service_pattern}"
```

### Schema requirements

Each saved alert filter has:

- Unique name.
- One or more matchers.
- Optional parameter definitions.

Each matcher has:

- `label`
- `operator`
- `value`

Supported operators:

`=`, `!=`, `=~`, `!~`

No alternate matcher schema is needed.

Do not introduce separate `value` and `regexp` fields.

The operator determines matcher semantics.

### Validation

- Nonempty filter name.
- Case-insensitive unique names.
- Valid Prometheus label names.
- Supported operators.
- Valid parameter declarations.
- All parameter references defined.
- No duplicate parameter declarations.
- At least one matcher.
- Valid regex after parameter resolution.
- Clear errors for malformed YAML.

Do not silently discard invalid entries.

### Matcher evaluation

Saved Alerts filters operate on alert labels.

Use Alertmanager-compatible matcher semantics, including:

- Full-string regex matching.
- Negative matchers.
- Missing-label behavior.
- Logical AND across matchers.

Do not implement a different approximation of matcher semantics.

Reuse existing matcher implementations where appropriate, especially if the Silence Management milestone already contains compatible code.

### Filtering architecture

Saved Alerts filters are applied to the existing fetched Alertmanager alerts.

Do not introduce a separate API request per saved filter.

The processing pipeline should conceptually be:

```text
Alertmanager API
       ↓
Fetched alerts
       ↓
Structured label matchers
       ↓
Existing status filters
       ↓
Existing fuzzy search
       ↓
Sorting and rendering
```

Preserve the current status-filter, fuzzy-search and sorting implementations.

### Important behavior

A saved alert filter contains **only label matchers**.

It must not save:

- Alert status filters.
- Fuzzy-search text.
- Sort order.
- Current connection.
- Selected alert.
- UI layout state.

Saved filters are global across connections.

Applying a saved filter to another connection evaluates the same matchers against that connection's alerts.

A saved filter is not an Alertmanager silence and must never modify server state.

## 8. Manual matcher editor

Introduce an Alerts matcher editor that can be used independently of saved filters.

Users must be able to:

- Add a matcher.
- Edit a matcher.
- Remove a matcher.
- Choose an operator.
- Enter a value.
- Apply the matcher set.
- Clear the matcher set.
- Save the current matcher set.

Use the same underlying matcher model and evaluator as saved filters.

Do not build separate implementations for manual and saved filtering.

### UI

Use a true floating overlay.

Example:

```text
┌─ Alert label matchers ──────────────────────────────┐
│                                                    │
│ Label          Operator   Value                    │
│ alertname      =~         .*HAProxy.*              │
│ servicename    =          okd4-prod                │
│                                                    │
│ Add matcher · Remove matcher                       │
│                                                    │
│ Ctrl+Enter Apply                 Esc Cancel         │
└────────────────────────────────────────────────────┘
```

Use existing Bubble Tea patterns and contextual shortcuts.

Do not interfere with the Alerts list's existing `s` shortcut for sorting.

### Active filter state

The Alerts tab should clearly indicate when structured label matchers are active.

Example:

`Alerts | HAProxy by service (service=okd4-prod)`

For manually created matchers, show an appropriate unsaved-filter indicator.

Provide a straightforward action to clear the active matcher filter.

Clearing the structured filter must not clear fuzzy search or status filters.

## 9. Saved Alerts management

Reuse the Saved Queries interaction conventions.

Suggested shortcuts within Alerts:

| Shortcut | Action |
|---|---|
| `Ctrl+O` | Browse saved alert filters |
| `Ctrl+S` | Save current matcher filter |
| `Enter` | Load selected saved filter |
| `e` | Rename saved filter in browser |
| `u` | Update saved filter |
| `d` | Delete saved filter |
| `/` | Search saved filters |
| `Esc` | Cancel / close |

Choose a contextual shortcut for opening the manual matcher editor.

Avoid collisions with existing Alerts shortcuts.

### Saved filter browser

Support:

- Listing.
- Fuzzy search.
- Selection.
- Loading.
- Rename.
- Update.
- Delete.
- Confirmation for destructive operations.
- Clear validation errors.
- Empty state.

### Parameterized filters

Loading a parameterized filter opens the shared parameter dialog.

On confirmation:

1. Resolve parameters.
2. Validate final matchers.
3. Apply the resolved matcher set.
4. Update the active-filter indicator.

On cancellation or validation failure, preserve the previous active filter.

### Save and update semantics

When saving a manual matcher set, persist its literal values.

When updating a parameterized saved filter, preserve its template and parameter definitions unless the user explicitly edits that template.

**Do not accidentally overwrite `${service}` with the last resolved value.**

A loaded filter has both:

- Its saved template definition.
- Its currently resolved runtime matcher values.

Keep these distinct.

Provide an explicit, understandable path for editing a saved filter's matcher templates and parameter definitions. Do not require users to reconstruct templates from resolved values.

## 10. Saved Alerts persistence

Requirements:

- Create the file when needed.
- Handle missing file as an empty collection.
- Preserve valid existing entries.
- Atomic writes.
- Safe permissions consistent with existing Saved Queries.
- No partial writes on validation failure.
- Deterministic serialization.
- Clear errors on permission or filesystem failures.
- Avoid overwriting externally modified content silently.

### External generation

`alerts.yaml` is expected to be generated or managed externally in some environments.

Keep the schema simple.

Do not introduce:

- Internal IDs.
- Connection-specific metadata.
- Runtime state.
- UI state.
- Generated timestamps.
- Polymorphic item types.

The application should load externally generated valid YAML without requiring it to be rewritten first.

## 11. Phase C — Parameterized Saved PromQL

Extend the existing Saved Queries schema with optional parameters.

Example:

```yaml
queries:
  - name: CPU usage
    expression: |
      rate(node_cpu_seconds_total{mode="idle"}[5m])

  - name: CPU usage by instance
    parameters:
      - name: instance
        default: server01:9100
    expression: |
      rate(node_cpu_seconds_total{
        instance="${instance}",
        mode="idle"
      }[5m])

  - name: Request rate by service
    parameters:
      - name: service
    expression: |
      sum by (instance) (
        rate(http_requests_total{
          servicename="${service}"
        }[5m])
      )
```

### Backward compatibility

The existing `queries.yaml` schema must remain valid.

Do not require a version field or file migration.

Existing saved queries without parameters must behave exactly as before.

### Loading behavior

For a saved query without parameters:

- Load immediately.
- Preserve existing behavior.

For a saved query with parameters:

1. Open the shared parameter dialog.
2. Prefill defaults.
3. Request missing values.
4. Resolve the expression.
5. Load the resolved expression into the PromQL editor.
6. Do not execute automatically.

Preserve:

- Current connection.
- Query mode.
- Time range.
- History.
- Existing results until a new query is executed.

### Parameter semantics

Use the same raw substitution engine as Saved Alerts.

Do not add automatic PromQL escaping.

Do not add parameter types.

Do not add a second placeholder syntax.

Do not attempt to parse MetricsQL using a Prometheus-only parser as a mandatory validation gate.

### Editing and updating

Preserve the distinction between the saved expression template and the resolved editor expression.

For example, loading:

`rate(metric{instance="${instance}"}[5m])`

with `instance=server01:9100` must not silently change the stored template to:

`rate(metric{instance="server01:9100"}[5m])`

when the user later updates the saved query.

Define explicit update behavior so the user can intentionally edit either the template or the resulting expression without accidental template destruction.

Maintain compatibility with existing `Ctrl+S`, `Ctrl+O`, rename, update and delete workflows.

## 12. Shared UI integration

The Saved Queries and Saved Alerts interfaces should look and behave consistently.

Reuse:

- Floating overlay patterns.
- Search behavior.
- Name editing.
- Confirmation dialogs.
- Parameter prompting.
- Validation feedback.
- Footer conventions.
- Keyboard navigation.

However, do not force both features to use one enormous UI model if that complicates state handling.

Shared components should be small and composable.

### Focus and overlays

- `Esc` closes the topmost overlay.
- Keyboard events must not leak to underlying panels.
- Opening a parameter dialog must not execute queries or mutate filters.
- Cancel restores previous focus.
- Terminal resizing must not lose entered parameter values.
- Overlays must not reflow the underlying UI.
- Preserve existing tab-switching behavior.

## 13. Tests

### Shared engine

Test:

- No parameters.
- One parameter.
- Multiple parameters.
- Repeated references.
- Embedded references.
- Empty defaults.
- Missing defaults.
- Explicit empty values.
- Missing required values.
- Undefined references.
- Duplicate definitions.
- Invalid names.
- Malformed placeholders.
- Literal dollar signs.
- Single-pass substitution.
- Values containing placeholder-like text.
- Regex metacharacters preserved.
- Unicode parameter values.

### Saved Alerts

Test:

- Valid YAML.
- Invalid YAML.
- Missing file.
- Duplicate names.
- Invalid label names.
- Invalid operators.
- Invalid regex.
- Regex becoming invalid after substitution.
- Exact equality.
- Inequality.
- Regex matching.
- Negative regex matching.
- Full-string regex semantics.
- Missing labels.
- Multiple matchers combined with AND.
- Manual matcher editing.
- Saving.
- Loading.
- Renaming.
- Updating.
- Deleting.
- Parameterized loading.
- Cancelled parameter dialog.
- Preservation of saved templates.
- Filtering after refresh.
- Filtering after connection switching.
- Interaction with existing fuzzy and status filters.
- Atomic persistence.
- Externally generated YAML.

### Saved PromQL

Test:

- Existing files without parameters.
- Existing Saved Queries behavior.
- Parameterized expressions.
- Multiline expressions.
- Raw substitution.
- Missing required parameters.
- Default values.
- Cancelled loading.
- No automatic execution.
- MetricsQL expressions preserved.
- Template preservation on update.
- No unintended modification of history or query mode.

### UI regression

Test:

- Existing Alerts navigation.
- Existing Alerts sorting.
- Existing fuzzy search.
- Existing Saved Queries shortcuts.
- Overlay focus.
- Tab switching.
- Terminal resizing.
- Kitty graphics cleanup.
- OSC 7501 behavior.
- Connection switching.

## 14. Documentation

Update project documentation with:

- Shared parameter syntax.
- Raw substitution semantics.
- Parameter defaults.
- Required parameters.
- Literal dollar-sign escaping.
- `alerts.yaml` format.
- `queries.yaml` optional parameters.
- Manual Alerts matcher editor.
- Saved Alerts shortcuts.
- Saved PromQL parameter dialog.
- Filter clearing.
- Global saved-item behavior.
- External YAML generation.
- Validation errors and limitations.

Provide copyable YAML examples for both files.

Explicitly document that parameters are substituted without automatic regex or PromQL escaping.

## 15. Orchestration and delegation

You are responsible for planning and coordinating the implementation.

### Recommended sequence

**Stage 1 — Architecture inspection**

Inspect the repository, identify reusable code, and establish precise package boundaries.

**Stage 2 — Shared engine**

Implement Phase A first.

This establishes the interfaces and data structures that both consumers will use.

**Stage 3 — Independent consumers**

Once Phase A is stable, delegate:

- Saved Alerts implementation.
- Saved PromQL parameters implementation.

These may proceed in parallel if their file ownership and dependencies permit.

**Stage 4 — Integration**

Integrate both consumers, resolve shared UI and state conflicts, and run the complete test suite.

**Stage 5 — Review**

Conduct an independent code review, address findings and rerun tests.

**Stage 6 — Final merge**

Merge the completed implementation into the integration branch.

### Subagents

Use subagents where they materially improve execution.

Potential roles:

- Architecture and shared engine.
- Saved Alerts implementation.
- Saved PromQL implementation.
- Test and regression review.
- Documentation review.

Do not delegate blindly.

Provide each subagent with:

- Clear scope.
- Relevant architecture context.
- Shared engine interfaces.
- Expected tests.
- Files or packages it owns.
- Constraints on changing shared code.

Avoid concurrent edits to the same files where possible.

### Git worktrees

Worktrees are optional but encouraged for genuinely independent implementation branches.

Suggested structure:

```text
feature/saved-params-engine
feature/saved-alerts
feature/saved-promql-params
```

The orchestrator should determine the actual branch/worktree structure after inspecting the repository.

If the consumer implementations depend on Phase A, branch them from the completed engine baseline.

Do not create unrelated implementations of the shared engine in separate worktrees.

### Merge responsibility

The orchestrator must:

- Integrate the shared engine.
- Merge or cherry-pick the consumer implementations.
- Resolve conflicts.
- Review integration changes.
- Run tests after integration.
- Ensure no feature is accidentally omitted.
- Commit the integrated result.
- Perform the final merge into the original integration branch.

Do not ask the user to manually merge individual phases.

Do not force-push, discard uncommitted changes or overwrite unrelated work.

If the target branch has unrelated uncommitted changes, protect them and use a separate integration worktree rather than modifying them.

If a genuine unresolved conflict or destructive operation requires user intervention, report it clearly rather than making an unsafe assumption.

### Do not stop prematurely

Do not stop after:

- Writing a plan.
- Implementing Phase A.
- Creating subagent tasks.
- Completing only Saved Alerts.
- Completing only Saved PromQL.
- Producing unmerged branches.
- Running tests without integration.

Continue until all acceptance criteria are satisfied or a genuine blocker is encountered.

## 16. Quality gates

At each phase boundary:

- Format changed Go files.
- Run relevant unit tests.
- Review error handling.
- Review state management.
- Check backward compatibility.
- Commit a coherent change.

At final integration:

- Run `go test ./...`.
- Run the repository's existing lint/static-analysis checks, if configured.
- Run applicable integration tests.
- Review the final diff.
- Check for unintended dependencies.
- Verify documentation.
- Verify YAML examples.
- Verify no unrelated changes were introduced.

If practical, build the application and perform a short interactive smoke test.

Do not claim manual UI verification unless it was actually performed.

## 17. Non-goals

Do not implement:

- Parameter autocomplete.
- Dynamic label-value suggestions.
- Additional Alertmanager API calls for saved filters.
- Connection-specific saved filters.
- Status filters inside saved alert definitions.
- Saved fuzzy-search state.
- Saved sorting state.
- A combined polymorphic YAML file.
- A general-purpose template language.
- Environment-variable expansion.
- Recursive parameter substitution.
- Automatic regex escaping.
- Automatic PromQL escaping.
- PromQL or MetricsQL query rewriting.
- Service discovery.
- Rule editing.
- Alertmanager configuration modification.
- Backend vendor adapters.

## 18. Acceptance criteria

The task is complete when:

1. A shared parameter engine exists and is covered by tests.
2. Existing Saved Queries remain backward compatible.
3. `alerts.yaml` supports simple externally generated saved filters.
4. Saved Alerts supports all four matcher operators.
5. Saved Alerts supports raw parameter substitution.
6. Users can manually edit and apply Alerts label matchers.
7. Users can save, browse, load, rename, update and delete alert filters.
8. Saved filters do not alter fuzzy search, sorting or status-filter state.
9. Saved PromQL queries support optional parameters.
10. Both domains use the same parameter engine and reusable parameter dialog.
11. Parameterized templates are not accidentally overwritten by resolved values.
12. Future parameter autocomplete remains architecturally possible without being implemented.
13. All existing functionality remains operational.
14. Tests and documentation are complete.
15. All implementation branches are integrated.
16. The orchestrator has completed the final merge.

## 19. Final report

After completion, provide a concise report containing:

- Implemented features.
- Architectural changes.
- New or modified YAML formats.
- Subagents and worktrees used.
- Test results.
- Integration branch and final merge commit.
- Any limitations or remaining issues.

Do not report success for work that was not completed.

**Deliver the implementation, not just the plan.**

### Rex pane integration

The orchestrator should prefer **visible Rex panes for independently running coding agents**.

Before spawning agents, inspect the available Rex integration, commands and skills.

When supported:

- Create one pane per independent agent session.
- Give each agent its own Git worktree.
- Use descriptive pane/session titles, such as `saved-alerts` and `saved-promql`.
- Keep the orchestrator in the original pane.
- Allow me to inspect agent progress without switching OpenCode sessions.
- Reuse existing panes where appropriate.
- Avoid terminating panes containing unrelated user work.

Do not create separate panes for ordinary internal OpenCode subagent calls if the integration does not support that.

If Rex pane creation is unavailable, continue using OpenCode's normal agent delegation rather than blocking implementation.

The orchestrator remains responsible for collecting results, reviewing changes and performing the final merge.
