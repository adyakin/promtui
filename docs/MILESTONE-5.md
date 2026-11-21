# Milestone 5 — Alertmanager Integration

**1 Alerts** now loads and explores current alerts independently of PromQL. **0 PromQL** and **2 Targets** retain their existing behavior.

## Configuration and API

Set `alertmanager_url` on a `[[connections]]` entry in TOML. It is optional and never inferred from the query backend. For example:

```toml
[[connections]]
name = "cluster"
url = "http://vmselect:8481/select/0/prometheus"
alertmanager_url = "https://monitoring.example.org/alertmanager"
```

PromTUI appends `/api/v2/alerts` to the base URL, preserving reverse-proxy prefixes, and sends `active=true`, `silenced=true`, `inhibited=true`, and `unprocessed=true`. It reuses the default Go HTTP transport, including normal HTTPS verification, and the existing `[query].timeout`. This works the same way for Prometheus and VictoriaMetrics connections. Missing or invalid configuration is shown in Alerts without preventing PromQL from starting.

## Implemented

- Compact counts and a keyboard-scrollable table; a single alert can count as both silenced and inhibited. Narrow terminals show state and name, with all fields accessible in details.
- Interactive, case-insensitive substring filtering across label values. Sort by state/severity/oldest first, or switch to newest first. Selection survives sorting and refresh by source-scoped fingerprint or canonical labels; missing selections fall back to the nearest row.
- Floating details with wrapped annotations, sorted labels, receivers, suppression references, timestamps, and generator URL. **y** copies the complete labels. Contextual help and the footer match the active view.
- Manual refresh and a 30-second automatic refresh while visible. Leaving cancels an in-flight request; returning refreshes stale data. A failure replaces the list with the error in the Alerts pane; use the arrow keys to scroll long errors. **Alt+S** opens request status. A successful empty response shows `No active alerts`.

## Source files

- `internal/alertmanager/client.go`: wire format, request, and error handling.
- `internal/config/config.go`, `examples/config.example.toml`: connection configuration.
- `internal/app/alerts.go`, `alert_rows.go`, `alerts_view.go`: refresh, alert presentation, keyboard handling, table and overlays.
- `internal/app/shell.go`, `model.go`: tab routing and application integration.

## Verification

- **Automated:** `go test ./...` covers HTTP decoding, missing fields, empty results, auth and other HTTP errors, malformed JSON, timeouts, cancellation, a TLS reverse-proxy prefix, identity, summary counts, sorting, filtering, duration, overlay layout, footer, and refresh state. Existing PromQL, Targets shell, and Kitty graphics tests also pass.
- **Additional checks:** `go vet ./...` and `go test -race ./internal/alertmanager ./internal/app` passed.
- **Live Rex smoke test:** Used the configured Alertmanager to load active alerts, move selection, open and page through details, apply a filter, refresh manually, and switch to PromQL and back. The filter and selected alert persisted, and the update timestamp advanced on automatic refresh. This endpoint showed only unsuppressed alerts during the check; overlapping suppression states and failure handling were verified with automated fixtures. No pixel-level or clipboard verification was performed.

## Limitations and deferred work

Alertmanager has no native evaluation-value field; an explicitly supplied `value` annotation is shown as such. Details are a snapshot of the alert when opened. The app classifies HTTP authentication failures but does not configure credentials, auth headers, or custom TLS settings. Silence mutations, acknowledgements, routing and notifications, rule and pending-alert queries, history, and multi-Alertmanager aggregation are out of scope.

For the next milestone, reuse the shell’s independent request and focus state for the next backend view. Consider adding authenticated connection configuration separately if needed.
