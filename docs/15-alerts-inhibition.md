# PromTUI — Alertmanager Inhibition Explainability

## Goal

Extend the existing Alerts tab to explain why an alert is inhibited and which alerts are responsible for inhibiting other alerts.

PromTUI should make Alertmanager inhibition relationships understandable directly from the terminal, without requiring the user to inspect `inhibit_rules` manually or open the Alertmanager Web UI.

## 1. Data sources

Use the existing Alertmanager API client and connection configuration.

- `GET /api/v2/status` — obtain `config.original` and parse `inhibit_rules`.
- `GET /api/v2/alerts` — obtain current alerts, including inhibited alerts.
- Reuse the existing Alerts data and refresh lifecycle wherever possible.
- Do not introduce additional configuration or a separate API client.

Alertmanager may already provide `inhibitedBy` identifiers and `status.state` for alerts. Use these as authoritative runtime information where available, and use local rule evaluation to explain the relationship.

Do not assume that a locally matching inhibition rule necessarily caused the current runtime state.

## 2. Inhibition rule evaluation

Implement a small, independently testable evaluator supporting:

- `source_matchers` and `target_matchers`.
- Operators `=`, `!=`, `=~`, `!~`.
- The `equal` label list.
- Missing labels and empty values according to Alertmanager semantics.
- Full-string regular-expression matching.
- Multiple matching rules and multiple source alerts.
- Alertmanager's self-inhibition prevention behavior.

A rule matches a source/target pair when the source matchers match the source alert, target matchers match the target alert, and all `equal` labels have equal values.

Never treat an alert as inhibiting itself. Also respect Alertmanager's protection against mutual inhibition when alerts match both the source and target sides of a rule.

Use existing matcher parsing utilities where practical, but avoid coupling the evaluator to the UI.

## 3. Alert Details — inhibited alerts

When the selected alert is inhibited, add an **Inhibition** section.

Example:

```yaml
Inhibition:
  State: inhibited

  Source alert:
    alertname: HighLatency
    servicename: payments
    severity: critical

  Matching rule:
    source_matchers:
      - severity="critical"
    target_matchers:
      - severity="warning"
    equal:
      - alertname
      - servicename
```

The actual presentation should follow PromTUI's existing terminal styling, with readable labels, colors and scrolling.

Show all relevant source alerts and rules, not just the first match.

Allow the user to navigate to a source alert using a contextual keyboard shortcut. Reuse the existing Alerts selection/navigation mechanism.

## 4. Alert Details — inhibiting alerts

For an alert acting as an inhibition source, show an additional section:

```yaml
Inhibits:
  - alertname: HighLatency
    servicename: payments
    severity: warning

  - alertname: HighLatency
    servicename: checkout
    severity: warning
```

Show the number of currently inhibited target alerts.

Allow navigation to a target alert from this section.

If an alert is both inhibited and an inhibition source, show both relationships where supported by the available data.

## 5. Runtime truth vs local explanation

Clearly distinguish:

- **Confirmed inhibition:** Alertmanager reports the alert as inhibited, with runtime inhibitor references when available.
- **Matching rule:** PromTUI determines that a configured inhibition rule matches the source/target pair.
- **Potential inhibition:** A rule matches locally, but Alertmanager does not report the target as inhibited.

Do not present potential inhibition as confirmed inhibition.

If the Alertmanager configuration cannot be fetched or parsed, preserve the existing Alerts functionality and display a useful nonfatal error in the Inhibition section.

If runtime references cannot be resolved to fetched alerts, indicate that the source is unavailable rather than inventing an explanation.

## 6. UI and navigation

Follow the existing `docs/uix*.md` conventions.

- Keep inhibition information inside Alert Details.
- No new top-level tab.
- No extra permanent panel.
- Support multiple source and target alerts.
- Use contextual keyboard shortcuts and footer hints.
- Keep navigation reversible with `Esc`.
- Preserve current filters, search, sorting and selected connection when navigating between alerts.
- Avoid unnecessary API requests during navigation.

The UI should remain usable over ordinary SSH without Rex-specific features.

## 7. Performance

Inhibition evaluation should run when relevant alert or configuration data changes, not on every UI render.

Avoid repeated full pairwise scans during navigation.

For large alert sets, use reasonable indexing or precomputed relationships where useful, without introducing unnecessary complexity.

Retain the existing alert data if a background refresh fails.

## 8. Tests

Add unit tests covering:

- Critical alert inhibiting a warning alert with matching `alertname` and `servicename`.
- Different `servicename` values preventing inhibition.
- Missing or empty labels in `equal`.
- Regex and negative matchers.
- Multiple matching rules.
- Multiple source alerts inhibiting one target.
- One source inhibiting multiple targets.
- Self-inhibition prevention.
- Mutual source/target matcher overlap.
- Runtime inhibited state without resolvable source alerts.
- Locally matching rules without confirmed runtime inhibition.
- Missing, invalid or unavailable Alertmanager configuration.

Add appropriate UI/navigation tests where practical.

## 9. Documentation and completion

Update relevant project documentation and keyboard shortcut references.

Inspect the existing code and UI architecture before implementation. Reuse established patterns and avoid unrelated refactoring.

Run `go test ./...` and fix regressions.

Perform a final review focusing on Alertmanager inhibition semantics, runtime-state correctness, and terminal navigation.

## Acceptance criteria

The feature is complete when:

1. Selecting an inhibited alert explains which available source alerts and configured rules match it.
2. Selecting a source alert shows which targets it currently inhibits.
3. Users can navigate between related alerts without losing their Alerts view context.
4. PromTUI never incorrectly labels a locally inferred relationship as confirmed runtime inhibition.
5. Existing Alerts behavior remains unchanged when inhibition information is unavailable.
6. The feature works with the existing Alertmanager configuration and requires no new user settings.
