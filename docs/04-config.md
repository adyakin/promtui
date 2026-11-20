# Configuration — YAML, TOML, and TLS Settings

## Objective

Make YAML the preferred format, retain TOML compatibility, and introduce hierarchical TLS verification settings.

This is a configuration-only task. Do not implement Targets functionality or make unrelated UI changes.

Before implementation, inspect the existing configuration structures, loader, validation, tests, examples, and documentation.

Preserve all currently supported configuration options unless explicitly changed below.

## 1. Add YAML support alongside TOML

Use YAML for new configurations without requiring existing TOML users to migrate.

Requirements:

- Use `.yaml` as the canonical configuration extension.
- Update configuration loading, parsing, validation and error reporting.
- Preserve existing configuration semantics and defaults.
- Update default configuration paths and discovery logic.
- Update configuration examples and documentation.
- Update relevant tests and fixtures.

Use an established Go YAML library.

Do not introduce unnecessary configuration abstractions.

### Backward compatibility

- Discover `config.yaml`, then `config.yml`, then `config.toml`.
- Select the parser by extension for explicit `-config` paths.
- Report errors in the selected file instead of falling back.
- Preserve the TOML `[[connections]]` layout; YAML uses a mapping keyed by connection name. Both formats share defaults, validation, and TLS precedence.
- Do not automatically overwrite or convert existing user configuration files.

Update the documented default configuration path to use `.yaml`.

Preserve existing command-line configuration path overrides, if supported.

## 2. Global TLS Configuration

Introduce a global `insecure_skip_verify` setting.

Example:

```yaml
insecure_skip_verify: false
```

Default: `false`.

When enabled, HTTPS connections may skip TLS server certificate verification.

This setting applies to all configured connections unless explicitly overridden at the connection level.

Do not disable TLS verification globally through shared Go HTTP transport state.

## 3. Connection-Level TLS Override

Allow each connection to override the global setting.

Example:

```yaml
insecure_skip_verify: true

connections:
  production:
    url: https://victoriametrics.example.com
    alertmanager_url: https://alertmanager.example.com
    insecure_skip_verify: false

  development:
    url: https://victoriametrics-dev.example.com
    alertmanager_url: https://alertmanager-dev.example.com
    insecure_skip_verify: true

  staging:
    url: https://prometheus-staging.example.com
```

Effective behavior:

| Global | Connection | Effective |
|---|---|---|
| omitted | omitted | `false` |
| `false` | omitted | `false` |
| `true` | omitted | `true` |
| `false` | `true` | `true` |
| `true` | `false` | `false` |

**An explicitly configured connection value always overrides the global value, including `false`.**

Implementation must distinguish an omitted boolean from an explicitly configured `false`.

A `*bool` or equivalent optional value is appropriate for the connection configuration model.

## 4. TLS Policy Scope

The effective TLS policy belongs to the **connection**, not to individual URLs.

It must apply consistently to every HTTPS endpoint associated with that connection, including:

- Primary Prometheus/VictoriaMetrics query URL
- Alertmanager URL
- Future vmagent/Targets URLs
- Other future HTTP endpoints belonging to the same connection

Do not add per-endpoint `insecure_skip_verify` settings.

Future endpoint configuration should inherit the connection's effective TLS policy automatically.

### HTTP client handling

Ensure that:

- Each connection uses its resolved TLS configuration.
- HTTP transports do not leak TLS settings between connections.
- A connection with insecure TLS enabled cannot weaken another connection's verification.
- Existing timeout, authentication, proxy and HTTP settings continue working.
- TLS verification remains enabled by default.
- TLS verification is skipped only when explicitly configured.

Avoid modifying `http.DefaultTransport` or global TLS settings.

## 5. Configuration Resolution

Resolve the effective TLS setting centrally.

Conceptually:

```go
func effectiveInsecure(global bool, connection *bool) bool {
    if connection != nil {
        return *connection
    }
    return global
}
```

The resolved value should be used when constructing HTTP clients for the connection.

Avoid duplicating precedence logic across the PromQL, Alerts and future Targets implementations.

Keep raw configuration values separate from resolved settings where useful.

## 6. Validation and Errors

Provide clear errors for:

- Invalid YAML syntax
- Invalid configuration types
- Invalid URLs
- Missing required fields
- Unsupported configuration values

Preserve existing validation behavior wherever possible.

Do not silently ignore malformed TLS settings.

For example:

```yaml
insecure_skip_verify: "maybe"
```

Must produce a configuration error rather than silently falling back to `false`.

## 7. Tests

### YAML configuration

Test:

- Valid YAML loading
- Existing settings preserved
- Default values
- Invalid YAML
- Invalid configuration types
- Missing required values
- Explicit configuration path handling
- TOML compatibility and discovery priority

### TLS precedence

Test all combinations of global and connection-level values.

Explicitly verify:

- Global `true` + connection omitted = `true`
- Global `true` + connection `false` = `false`
- Global `false` + connection `true` = `true`
- Both omitted = `false`

### TLS integration

Use local HTTPS test servers with self-signed certificates.

Verify:

1. Verification enabled rejects an untrusted certificate.
2. Verification disabled permits the connection.
3. Global insecure configuration is inherited.
4. Connection-level `false` overrides global `true`.
5. Connection-level `true` overrides global `false`.
6. Different connections maintain independent TLS behavior.
7. Query and Alertmanager clients use the same effective connection policy.

Do not depend on external services for automated tests.

## 8. Documentation

Update:

- Configuration examples
- README setup instructions
- Configuration reference
- Relevant existing milestone documentation where necessary

Read all applicable `docs/uix*.md` files if any UI-facing configuration behavior is affected.

Do not rewrite historical milestone documents unnecessarily.

Document how to switch from TOML to YAML and the TLS precedence behavior in the [configuration reference](configuration.md).

## 9. Acceptance Criteria

The task is complete when:

- YAML is the preferred configuration format, and existing TOML configs remain supported.
- Existing configuration functionality is preserved.
- Global `insecure_skip_verify` works.
- Connection-level overrides work in both directions.
- The effective TLS policy applies to all URLs within a connection.
- Different connections cannot accidentally share insecure TLS settings.
- PromQL and Alertmanager functionality remain operational.
- Existing tests are updated and new TLS tests pass.
- Documentation and examples use YAML.
- No Targets implementation or unrelated refactoring is included.

## 10. Implementation Report

After implementation, summarize:

- Configuration files and structures changed
- New default configuration path
- TOML compatibility and discovery behavior
- TLS precedence implementation
- HTTP client changes
- Tests executed and results
- Any compatibility limitations

Do not automatically proceed to Targets implementation.
