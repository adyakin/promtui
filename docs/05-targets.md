# Milestone 6 — Scrape Targets Explorer

**Project:** PromTUI
**Scope:** Targets tab, Prometheus-compatible Targets API, multiple scrape sources, filtering, details and refresh

## 1. Objective

Replace the existing Targets placeholder with a functional, keyboard-driven scrape target explorer.

Users should be able to open tab `2` and immediately see scrape target health across all configured sources.

Support:

- Prometheus scraping its own targets
- VictoriaMetrics installations using one or more vmagent instances
- Multiple independent Prometheus or vmagent sources within a single connection

Do not introduce a backend `type` or `mode` field.

Before implementation, inspect the current architecture, configuration loader and all applicable `docs/uix*.md` files. Follow the established keyboard, layout, floating overlay and context-sensitive footer conventions.

## 2. Configuration

Extend the existing YAML connection configuration with an optional list of named target sources.

Example:

```yaml
insecure_skip_verify: false

connections:
  production:
    url: https://victoriametrics.example.com
    alertmanager_url: https://alertmanager.example.com
    insecure_skip_verify: true

    targets:
      - name: moscow
        url: https://vmagent-msk.example.com

      - name: novosibirsk
        url: https://vmagent-nsk.example.com

      - name: spb
        url: https://vmagent-spb.example.com

  staging:
    url: https://prometheus.example.com
    alertmanager_url: https://alertmanager-staging.example.com
```

### Source resolution

- If `targets` is explicitly configured, use exactly those sources.
- If `targets` is omitted, use the connection's primary URL as a single default source.
- Do not infer source URLs from the backend hostname.
- Do not attempt automatic vmagent discovery.
- Do not introduce backend-specific target discovery modes.
- If the default endpoint does not support the Targets API, display a clear error suggesting explicit source configuration.

An explicitly configured empty `targets` list should produce a configuration validation error rather than silently falling back to the primary URL.

### Validation

- Source names must be unique within a connection.
- Source names and URLs must be nonempty.
- URLs must use supported HTTP(S) schemes.
- Preserve URL path prefixes for reverse-proxied sources.
- Preserve existing configuration behavior and compatibility.

### TLS

Use the effective connection-wide `insecure_skip_verify` setting implemented in the previous task.

Every target source within a connection must inherit the same TLS policy.

Do not introduce per-source TLS overrides.

## 3. Targets API

Use the Prometheus-compatible endpoint:

```http
GET /api/v1/targets?state=any
```

This API is supported by Prometheus and vmagent.

Request both active and dropped targets.

### Active targets

Parse:

- `discoveredLabels`
- `labels`
- `scrapePool`
- `scrapeUrl`
- `globalUrl`, if available
- `lastError`
- `lastScrape`
- `lastScrapeDuration`
- `health`
- `scrapeInterval`, if available
- `scrapeTimeout`, if available

### Dropped targets

Parse dropped target information where available.

Dropped targets do not have the same scrape health semantics as active targets.

Do not classify dropped targets as DOWN.

### API compatibility

Handle missing optional fields gracefully.

Do not assume that every Prometheus-compatible implementation returns identical optional fields.

Keep API decoding separate from UI presentation models.

## 4. Multi-Source Collection

Query all configured sources independently.

Requirements:

- Fetch sources concurrently with bounded concurrency.
- Apply existing HTTP timeouts and cancellation behavior.
- Preserve source identity for every returned target.
- Allow partial success.
- Do not fail the entire Targets tab because one source is unavailable.
- Do not silently omit failed sources.

### Source status

Track per-source state:

- Loading
- Healthy
- Error
- Stale

A healthy source can contain DOWN targets.

An unavailable source does not mean its targets are DOWN.

### Cached data

When a source refresh fails:

- Preserve its last successful data.
- Mark that data as stale.
- Show the source error.
- Exclude stale data from current-health totals, or clearly distinguish it from fresh data.

When a source recovers, replace its stale data with the new response.

## 5. Targets Tab Layout

Use the existing application shell and visual styling.

Do not introduce a new UI framework or redesign existing tabs.

### Summary

Show compact aggregate statistics:

```text
Targets: 342 UP · 8 DOWN · 15 DROPPED | Sources: 3/3 OK
```

Counts must come from actual data.

Clearly distinguish source failures from target failures.

If any source is unavailable, indicate that aggregate counts are incomplete.

### Main table

Suggested columns:

| Column | Content |
|---|---|
| Health | UP / DOWN / UNKNOWN |
| Source | Configured source name |
| Job | `job` label |
| Instance | `instance` label |
| Last Scrape | Relative time |
| Duration | Last scrape duration |

Default ordering:

1. DOWN
2. UNKNOWN
3. UP

Within each group, sort by source, job and instance.

Use clear visual distinctions for target health without relying exclusively on color.

### Dropped targets

Support active and dropped target views.

Default view: active targets.

Dropped targets should be accessible through a lightweight filter or view toggle.

Do not mix dropped targets into the main health ordering as though they were actively scraped.

### Terminal width

The table must handle narrow terminals.

Prioritize:

1. Health
2. Instance
3. Job
4. Source
5. Last scrape
6. Duration

Hide less important columns as needed.

Support vertical scrolling for large target lists.

## 6. Source Selector

Introduce a source filter opened with `f`.

Use the existing true floating overlay system.

Example:

```text
╭─ Target Sources ─────────────╮
│                              │
│  ● All sources               │
│    moscow                    │
│    novosibirsk               │
│    spb                       │
│                              │
╰──────────────────────────────╯
```

Requirements:

- `All sources` selected by default.
- `↑ / ↓` navigate.
- `Enter` selects a source.
- `Esc` closes without changing selection.
- Filtering uses already cached data.
- Changing source does not trigger an API request.
- Source selection persists across tab switches and refreshes.

Display source status and target counts where practical.

## 7. Filtering and Sorting

### Text filtering

Press `/` to filter targets.

Search case-insensitively across:

- Source name
- Job
- Instance
- Scrape URL
- Target label names and values

Filtering must not trigger API requests.

Preserve filter state across refreshes and tab switches.

### Health filtering

Provide a simple way to display:

- All active targets
- DOWN only
- UP only
- UNKNOWN only
- Dropped targets

Avoid introducing a complex filter expression language.

### Sorting

Support at least:

- Health
- Source
- Job
- Instance
- Last scrape duration

Sorting must remain deterministic.

Preserve the selected sort order across refreshes.

## 8. Target Details Overlay

Press `Enter` on a target to open a true floating overlay.

Follow all applicable `docs/uix*.md` requirements.

Do not resize or reflow the underlying table.

### Overview

Display:

- Source
- Health
- Job
- Instance
- Scrape pool
- Scrape URL
- Last scrape
- Last scrape duration
- Scrape interval and timeout, if available
- Last scrape error

### Labels

Display complete target labels, sorted by name.

### Discovered labels

Display the original discovered labels separately from the final target labels.

These are particularly useful when troubleshooting relabeling.

### Dropped targets

For dropped targets, display the available discovery information.

Do not invent scrape health, last scrape or error values.

### Interaction

- `↑ / ↓` scroll.
- `PgUp / PgDn` scroll by page.
- `y` copies target labels.
- `Esc` closes.

Long URLs, errors and labels must wrap or scroll without breaking the layout.

## 9. Keyboard Navigation

Preserve existing top-level navigation.

| Key | Action |
|---|---|
| `↑ / ↓` | Select target |
| `PgUp / PgDn` | Scroll table |
| `Home / End` | First / last target |
| `Enter` | Target details |
| `/` | Text filter |
| `f` | Source selector |
| `h` | Health filter |
| `s` | Sort |
| `d` | Toggle active/dropped view |
| `r` | Refresh |
| `Esc` | Close overlay/cancel |
| `?` | Help |

Use existing key conventions where they conflict with these suggestions.

The footer must remain context-sensitive and display only relevant actions.

## 10. Refresh Behavior

Default auto-refresh interval: 30 seconds.

Requirements:

- Refresh while Targets tab is active.
- Refresh stale data when returning to Targets.
- Support manual refresh with `r`.
- Avoid overlapping requests for the same source.
- Cancel or ignore obsolete requests.
- Preserve selected target, filters, sorting and source selection.
- Avoid table flickering.
- Keep existing data visible while refreshing.

### Selection identity

A target's identity must include its source.

Do not use only `instance` or `job`.

Use stable target metadata and labels to preserve selection across refreshes.

For dropped targets, use available discovery labels and source identity.

Handle duplicate identities deterministically.

## 11. Error and Empty States

Distinguish:

- No active targets
- No targets matching the current filter
- No dropped targets
- Missing/invalid configuration
- Unsupported API endpoint
- Authentication failure
- Connection timeout
- Partial source failure
- All sources unavailable

Do not show `0 DOWN` as proof of healthy scraping when sources could not be queried.

When some sources fail, continue showing healthy-source results and make the partial failure obvious.

## 12. Architecture

Suggested conceptual structure:

```text
Targets View
├── Source Configuration
├── Targets API Client
├── Multi-Source Collector
├── Target Data Model
├── Refresh Controller
├── Filtering / Sorting
├── Targets Table
├── Source Selector Overlay
└── Target Details Overlay
```

Reuse existing HTTP client infrastructure and connection TLS resolution.

Avoid unnecessary generic abstractions.

The same Targets API implementation should work with Prometheus and vmagent.

## 13. Testing

### Configuration tests

- Explicit multiple sources
- Default source fallback
- Empty source list validation
- Duplicate source names
- Invalid URLs
- Reverse-proxy path prefixes
- Connection-wide TLS inheritance

### API tests

Use mock HTTP servers.

Test:

- Prometheus-style response
- vmagent-style response
- Active targets
- Dropped targets
- Missing optional fields
- Malformed responses
- HTTP failures
- Timeouts and cancellation

### Multi-source tests

- All sources succeed
- One source fails
- All sources fail
- Failed source recovers
- Stale cache preservation
- Independent source status
- No duplicate refreshes

### UI tests

- Table navigation
- Source filtering
- Text filtering
- Health filtering
- Sorting
- Details overlay
- Selection preservation
- Tab switching
- Narrow terminal layout
- Context-sensitive footer

### Manual Rex verification

Verify with a real Prometheus instance and a VictoriaMetrics installation using multiple vmagent sources.

Pay particular attention to large target lists, long scrape errors and overlays.

## 14. Non-Goals

Do not implement:

- Editing scrape configurations
- Target relabeling configuration changes
- Service discovery management
- Prometheus/vmagent configuration reload
- Automatic vmagent discovery
- Backend mode/type configuration
- Historical target health
- Alertmanager integration changes
- Additional top-level tabs

## 15. Documentation

Create `docs/MILESTONE-6.md` containing:

- Implementation summary
- Configuration examples
- Multi-source behavior
- API compatibility notes
- Source health vs target health
- TLS handling
- Keyboard shortcuts
- Tests and results
- Known limitations
- Deferred functionality

Review all existing `docs/uix*.md` files before implementation and follow their established conventions.

Do not automatically proceed to another milestone.

## 16. Acceptance Criteria

Milestone 6 is complete when:

- Targets tab displays real scrape targets.
- Prometheus works without explicit target sources.
- Multiple vmagent sources work under one connection.
- Target and source health are distinguished correctly.
- Active and dropped targets are supported.
- Source, health and text filtering work.
- Target details appear in a floating overlay.
- Refresh preserves UI state.
- Partial source failures are handled correctly.
- Connection-wide TLS configuration is respected.
- Existing PromQL and Alerts tabs remain functional.
- Documentation and tests are complete.
