# Milestone 2 — PromQL editor

Run `make run` in Rex. Try `node_cpu` for metric names, `node_cpu_seconds_total{inst` for labels, or `node_cpu_seconds_total{instance="192.` for instance values. Suggestions use the labels returned by the active backend, including relabeled instance names.

| Key | Action |
|---|---|
| Ctrl+Enter / Enter | Execute / insert a newline. Ctrl+G and F5 still execute on terminals that cannot distinguish modified Enter. |
| Ctrl+Space / ↑↓ / Tab | Open suggestions / select / replace the token at the cursor. Typing a prefix also triggers suggestions after a pause. |
| Ctrl+R | Search this connection’s history by fuzzy subsequence. Enter restores a query for editing; Esc preserves the draft. |
| Alt+Up / Alt+Down | Recall older / newer history entries. |
| Esc | Dismiss a popup first; otherwise switch editor/results focus. |
| `?` / F1 | Show keys. `?` remains literal inside strings, comments and the history filter; F1 works there too. |
| Ctrl+C | Cancel the running query, or quit when idle. |

## Implementation

- Bubble Tea and Bubbles v2 support enhanced keyboard input. Tea negotiates key disambiguation and restores terminal modes. No raw-input shim is needed.
- `internal/editor` keeps syntax styling separate from editable text. A tolerant tokenizer recognizes strings, comments, delimiters, numbers, functions and keywords even in incomplete expressions. Matching brackets are emphasized at the cursor. The Bubbles textarea owns edits; a horizontally scrolling viewport paints source lines and the insertion cursor. The Prometheus parser reports line/column diagnostics after a typing pause. Diagnostics are advisory: Ctrl+Enter still sends backend-specific expressions unchanged.
- `internal/app/assist.go` owns completion and history popups. Local candidates include stable parser functions, operators, aggregations, and common range durations. Cursor context determines whether to suggest metrics, label names (including grouping clauses), or matcher values. Accepting a candidate replaces the token, preserving following text.
- `internal/api/metadata.go` uses `/labels` and `/label/<name>/values` (`__name__` for metrics). Prefix matchers narrow metric/value queries server-side; selectors are scoped to the metric when known. No `/series` enumeration or unscoped empty-prefix value lookup. Each lookup has a deadline of at most 3 seconds, a limit of 80 candidates and a 256 KiB body cap. Older commands are cancelled on edit/dismissal, and generation checks reject stale replies. The active connection caches up to 64 lookups for one minute.
- History search ranks the existing local JSONL entries, filtered by connection name. Searching, restoring and cancelling do not execute a query or add a history entry.

## Verification and limits

The Rex key probe received `0d` for Enter and `ESC [ 13 ; 5 u` for Ctrl+Enter under enhanced keyboard mode. In the app, Ctrl+Space → Tab completed `vec` to `vector`; Ctrl+Enter ran `vector(42)` against local Prometheus; Ctrl+R matched `v42`. A scoped value lookup for `up{job="mile` suggested the live target’s `milestone2` job while the parser correctly reported the unfinished string.

Run `make test` for offline coverage or `PROMTUI_TEST_URL=http://localhost:9090 go test -race ./...` to include live query checks. Tests cover unfinished selector contexts, quote/comment boundaries, matching brackets, metadata paths and filters, stale completions, cache hits, mid-token replacement, and fuzzy restore.

Completion is prefix-based and bounded, not an exhaustive catalog. It targets classic PromQL metric and label identifiers; quoted UTF-8 identifiers, full regex-aware prefix completion and MetricsQL-specific syntax are not covered. The table remains the results view; graph integration is Milestone 3.
