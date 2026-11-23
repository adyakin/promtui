# PromTUI — Routing Explorer: Raw Receiver YAML

## Objective

Replace the current integration-specific receiver details with the receiver's YAML configuration extracted from Alertmanager's `/api/v2/status` response.

The source of truth is:

```http
GET /api/v2/status
```

Specifically:

```json
{
  "config": {
    "original": "..."
  }
}
```

**Display what Alertmanager returns. Do not attempt to reconstruct, enrich or reinterpret the receiver configuration.**

## 1. Receiver Details

Keep the existing Receivers view and its left-hand list unchanged.

Replace the current integration details in the right-hand panel with the selected receiver's YAML.

Example:

```yaml
- name: jira-msdevops-bug
  jira_configs:
    - send_resolved: false
      http_config:
        authorization:
          type: Bearer
          credentials: <secret>
        follow_redirects: true
        enable_http2: true
      api_url: https://sd-jira.demo.env/rest/api/2/
```

Requirements:

- Extract the selected receiver from `config.original`.
- Display all fields returned by Alertmanager.
- Preserve field ordering.
- Preserve nested configuration.
- Support arbitrary integration types, including unknown or future types.
- Do not maintain integration-specific destination extraction.
- Do not replace redacted values with placeholders of our own.
- Do not omit fields because PromTUI doesn't recognize them.
- Do not display a separate `destination unavailable` message.

The right-hand panel should contain the receiver name, its YAML configuration and existing route references.

## 2. YAML Extraction

Use `gopkg.in/yaml.v3` and its `yaml.Node` representation.

Locate the top-level `receivers` sequence and find the mapping whose `name` matches the selected receiver.

Avoid decoding the receiver into integration-specific Go structs.

Preserve the receiver's complete YAML structure.

Prefer rendering the original YAML fragment where practical. If re-encoding the selected `yaml.Node` is necessary, preserving field order and values is more important than reproducing exact indentation.

Do not parse the configuration into Go structs and then serialize it again, as this could lose unknown fields.

## 3. Redaction

Alertmanager may return:

```yaml
credentials: <secret>
```

or:

```yaml
url: <secret>
```

Display those values exactly as returned.

Do not attempt to recover redacted information.

Do not read configuration files from the Alertmanager host.

Do not introduce additional APIs or permissions.

Do not add a second integration-specific redaction layer that removes fields or changes Alertmanager's returned values.

The configuration returned by `config.original` is the source of truth for this feature.

## 4. Navigation and Layout

Preserve the existing Routing Explorer behavior:

- Routes and Receivers views.
- Route → Receiver navigation.
- Receiver → Route navigation.
- Receiver reference counts.
- Search.
- Refresh.
- Connection switching.
- Existing keyboard shortcuts.

The YAML panel must support:

- Vertical scrolling.
- Horizontal scrolling or readable wrapping for long values.
- Terminal resizing.
- Copying the selected receiver's YAML using the application's existing clipboard conventions, if supported.

Keep the existing route references accessible below the YAML or in a separate section of the details panel.

Do not introduce a new top-level tab.

## 5. Route Details

Do not change the existing route tree or effective inheritance calculation.

As a small optional enhancement, display the selected route's original YAML fragment below its effective settings, using the same YAML-node approach.

If this would significantly expand the task, defer it. Receiver YAML is the primary requirement.

## 6. Error Handling

Handle:

- Missing `config.original`.
- Invalid YAML.
- Missing `receivers` field.
- Selected receiver not found.
- Empty receiver configuration.
- Refresh while a receiver is selected.
- Connection switching.

Show a clear error or unavailable state rather than crashing.

Do not replace the previous valid configuration after a failed refresh.

## 7. Tests

Add tests covering:

- Webhook receiver with redacted URL.
- Jira receiver with `api_url` and redacted Bearer credentials.
- Telegram receiver.
- Multiple integrations in one receiver.
- Unknown integration type.
- Arbitrary nested YAML fields.
- Preservation of field ordering.
- Preservation of `<secret>` values.
- Receiver selection and navigation.
- Missing and malformed configuration.
- Refresh and connection switching.

Include the Jira example above as a regression fixture.

## 8. Cleanup

Remove obsolete integration-specific destination extraction and rendering code if it is no longer used.

Retain any integration counting logic needed by the receiver list.

Update documentation to describe raw YAML display.

Avoid unrelated refactoring.

## Acceptance Criteria

1. Selecting a receiver shows its YAML from `config.original`.
2. All fields returned by Alertmanager are visible.
3. Unknown integration types work without code changes.
4. `<secret>` values appear unchanged.
5. No `destination unavailable` placeholders remain.
6. Existing bidirectional route/receiver navigation works.
7. Receiver references remain visible.
8. YAML is scrollable and readable.
9. Tests pass.
10. No other PromTUI features regress.
