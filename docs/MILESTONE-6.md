# Milestone 6 — Scrape Targets Explorer

Press `2` to explore active and dropped scrape targets. The table starts with
all active targets, ordered DOWN, UNKNOWN, then UP; ties are sorted by source,
job, and instance. Details float over the table and show final and discovered
labels separately, along with scrape metadata where available.

## Configuration

```yaml
connections:
  local:
    url: http://localhost:9090
  production:
    url: http://vmselect:8481/select/0/prometheus
    targets:
      - {name: east, url: "https://monitoring.example.org/vmagent-east"}
      - {name: west, url: "https://monitoring.example.org/vmagent-west"}
```

Omit `targets` to use the connection URL (`local` in this example). If the
default endpoint does not expose the Targets API, configure explicit sources.
The YAML `targets` list must not be empty, and each source must have a unique
name and an HTTP(S) base URL. Explicit sources replace the fallback. Legacy
TOML configs continue to use their connection URL. All sources inherit the
effective connection `insecure_skip_verify` setting; verification is on by
default. See the [configuration reference](configuration.md).

## Collection and source health

- Prometheus and vmagent use `GET /api/v1/targets?state=any`, with any
  reverse-proxy path prefix intact. Missing optional fields display as `—`
  or are omitted. Dropped targets use their discovery information, not scrape
  health.
- PromTUI fetches up to four sources at once, publishes results as they arrive,
  and applies `query.timeout` to each source. The tab refreshes at entry if
  needed, every 30 seconds while visible, or on demand. Tab or connection
  switches cancel running requests and ignore obsolete results.
- An **OK source** answered successfully; it may still have **DOWN targets**.
  On failure, a source with cached data becomes **Stale**. Its rows remain
  visible and are marked `*`, but do not count toward current totals. Failed
  sources make totals incomplete, and totals are unavailable if no source
  succeeded. Press `f` and highlight any source to inspect its status and
  error. A successful retry replaces the stale data.
- Filtering and sorting run on cached data. Selection, filters, and sort
  survive refreshes and tab switches. A connection switch starts a new view.

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `↑` / `↓`, `PgUp` / `PgDn`, `Home` / `End` | Navigate the table or scroll details |
| `Enter` | Open target details |
| `y` | Copy final target labels from details |
| `/` | Filter by source, job, instance, scrape URL, or label names/values; Enter applies, Esc cancels |
| `f` | Open source selector; Enter applies, Esc cancels |
| `h` | Cycle active, DOWN, UP, UNKNOWN, dropped |
| `d` | Toggle between dropped and the previous active view |
| `s` | Cycle health, source, job, instance, and longest scrape duration |
| `r`, `Ctrl+C` | Refresh now, or cancel a refresh in progress |
| `Esc`, `?` / `F1` | Close the overlay, or open help |

## Verification and limitations

`go test ./...` and `go vet ./...` pass. Tests cover configuration and TLS inheritance, compatible API
responses, timeouts and cancellation, bounded collection, partial and total
failures, stale cache recovery, and keyboard navigation and overlays at
normal and narrow terminal widths. Race checks passed for the API, app, and
config packages.

A Rex smoke test used Prometheus 3.16.0 and VictoriaMetrics 1.156.0 with two
vmagent instances. It exercised the default Prometheus source, a scrollable
46-target list, a dropped target, a floating details view with a wrapped
scrape error, and a partial failure that kept the stopped agent's rows stale
while excluding them from aggregate totals.

Targets are in-memory snapshots of what each scrape API returns. Some
deployments cap the number of retained dropped targets. PromTUI does not
discover vmagent instances, keep health history, edit scrape or relabeling
configuration, manage service discovery, or reload backends.
