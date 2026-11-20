# Milestone 1 — Working query explorer

The deliverable is a single Go TUI that executes multiline PromQL and lets you inspect the returned series and samples. It uses Bubble Tea's normal alternate-screen renderer and terminal lifecycle. No Spike 0 code, image transport, or graphics library is needed to run it.

## Run and configure

```sh
go run ./cmd/promtui
go run ./cmd/promtui -config config.example.toml -connection cluster -range 15m
go run ./cmd/promtui -start 2026-11-18T09:00:00Z -end 2026-11-18T10:00:00Z -step 30s
```

Start without configuration for `http://localhost:9090`. For named backends, copy `config.example.toml` to `~/.config/promtui/config.toml` on macOS or Linux. History defaults to `~/.local/share/promtui/history.jsonl`; override with `-history`. `[general].history_limit = 0` disables history. Set `[query].timeout`, `default_range`, and `max_series` in TOML. Duration options use Go syntax (`168h` for seven days). Connection changes and absolute times are startup options.

Try `vector(42)` against a running backend even if it hasn't scraped any targets. For instant queries `-end` fixes the evaluation time; otherwise each execution evaluates at the current time. For range queries it anchors the window. Automatic step targets about 240 intervals, with a one-second minimum; an explicit `-step` overrides it.

## Interaction

| Keys | Action |
|---|---|
| Enter / Ctrl+G or F5 | Insert newline / execute. No reliance on Ctrl+Enter encoding. |
| Ctrl+T / Ctrl+W | Toggle instant/range / select next relative range (15m, 1h, 6h, 24h, 7d). Execute to apply. |
| Alt+Up/Down or Ctrl+P/N | Recall older/newer queries for this connection in the editor. Moving past the newest restores the draft. |
| Tab or Esc | Switch editor/table focus. |
| Up/Down, PgUp/PgDn / Left/Right | Select series or page vertically / pan columns. |
| `[` / `]` | Inspect the previous/next sample of the selected series. Each row starts at its newest sample. |
| `l` / `n` / `v` | Toggle labels / sort by name ascending / sort by latest numeric value descending. |
| `y` / `Y` | Copy selected value / labels to the host clipboard. |
| F1 / F2 | Show keys / read full request status, result identity and warnings. Scroll with arrows; Esc closes. |
| Ctrl+C / Ctrl+Q | Cancel a running request (quit if idle) / quit, flushing pending history. |

## Code map and decisions

- `cmd/promtui/main.go` resolves paths, CLI overrides and the active connection. `internal/config` decodes TOML and validates settings before opening the terminal.
- `internal/app/model.go` owns focus, a Bubbles `textarea`, the last successful table, request identity and transient messages. HTTP, clipboard access, and history writes run as Tea commands. A replacement request cancels its predecessor; stale completions cannot change the UI. Errors leave the previous result visible with its query and mode. PromQL is sent as written: the backend supplies errors in its own dialect.
- `internal/api/client.go` issues form-encoded POSTs to `query` and `query_range`. It preserves a VictoriaMetrics tenant path or an existing `/api/v1` suffix, decodes HTTP/API errors and uses `prometheus/common/model` for scalar, string, vector and matrix payloads. Prometheus warnings/infos and VictoriaMetrics `isPartial` are surfaced. The response is capped at 32 MiB before JSON decoding.
- `internal/query/query.go` builds time parameters and normalizes responses into series with ordered samples, separate string values and stable label text. The series limit is sent to the backend and enforced locally for backends that ignore it.
- `internal/ui/table.go` formats just the visible rows. A matrix stays grouped by series rather than becoming thousands of table rows; sample navigation exposes the rest. This is why the table is a small viewport rather than a preformatted Bubbles table. Numeric samples retain their wire-model timestamps and float values; labels and string values are clipped by terminal cell width for display.
- `internal/history/history.go` stores newest-first JSONL with UTC timestamps, expression, and connection **name**. Re-execution promotes a duplicate. A single writer coalesces immutable snapshots and atomically replaces the private file, bounding both disk usage and recall length by entry count.

## Verification

```sh
go build ./cmd/promtui
go vet ./...
go test -race ./...
# Include integration tests against a running backend:
PROMTUI_TEST_URL=http://localhost:9090 go test -race -count=1 ./...
```

Verified locally with Go 1.27.4 on macOS/arm64 and Prometheus 3.5.0: instant scalar/string/vector results, range matrix values and sample count, empty results and invalid PromQL. HTTP fixtures cover tenant prefixes, partial results, warnings, non-JSON failure responses, timeouts, and a 10,000-series response ignoring the requested limit. Additional tests exercise table navigation, cancellation/stale completions, configuration defaults, history deduplication, draft restoration, and flushing coalesced writes on quit.

In Rex, a two-line `sum(vector(42))` executed with Ctrl+G. Switching to a one-hour range produced 241 samples; focusing the table and stepping backward showed `240/241`. Resizing the Rex terminal from 80×24 to 48×14 reduced editor height and clipped table columns. Relaunching with the same history file and pressing Ctrl+P restored and re-executed the multiline expression.

## Limits and handoff

- Native histogram samples are skipped with a notice. The bounded HTTP body is decoded in memory before local series limiting; this is not a streaming client. Extremely dense ranges should use a larger step. Reaching the series cap is reported even if the backend had exactly that many results.
- History is local plaintext, including PromQL literals. It stores no URLs or connection credentials. Simultaneous processes writing the same history path can overwrite each other's updates; use separate `-history` paths for concurrent sessions. A malformed history file produces a startup error rather than being overwritten.
- Clipboard support depends on the host clipboard helper. Live VictoriaMetrics, SSH, and clipboard integration were not verified here; VictoriaMetrics tenant paths and partial-response handling were tested with HTTP fixtures.
- The next planned milestone covers editor highlighting, parser diagnostics, metadata-backed completions and fuzzy history search. Metadata endpoints and a local PromQL parser belong there. Graph work remains a separate later milestone: revisit the Kitty placement, cleanup and lifecycle findings in `SPIKE-0.md` when a renderer actually needs them.
