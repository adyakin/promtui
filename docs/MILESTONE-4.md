# Milestone 4 — Multi-Tab Application Shell

PromTUI opens on **0 PromQL**. **1 Alerts** and **2 Targets** are single-panel placeholders that show the selected connection. They make no backend requests and contain no fake data. Prometheus and VictoriaMetrics continue to use the existing connection configuration and query client.

## Architecture and lifecycle

The Bubble Tea model separates `shellState` (active tab, terminal size, and shutdown state) from `promQLState` (editor, results, query settings, history, panels, and graph). The shell selects the visible workspace, draws the compact tab bar, and delegates footer actions to the active view. The placeholders each have a single, always-focused panel and no mutable state yet.

The PromQL state stays in memory when hidden. Returning restores the editor text and cursor, panel or overlay focus, results and view, selected and visible series, and time range. Switching tabs never runs or cancels a query. A query started in PromQL can finish in the background; its result is available on return. The inactive editor does not start new completion requests.

## Keyboard routing

- **0/1/2** select tabs outside text inputs. These digits are ordinary input in the PromQL editor, including during completion, and in the history search and series filter.
- **Alt+0/1/2** select tabs globally if the terminal forwards them. These keys are consumed before they reach an editor or filter.
- **Tab/Shift+Tab** stay within PromQL for Query/Results focus. **Tab** still accepts an open completion; **Esc** retains its overlay behavior and does not switch tabs.
- **Ctrl+C** cancels an active query or quits if idle; **Ctrl+Q** quits directly. **q** also quits from Results and the placeholders.

## Kitty graphics

Only the active PromQL workspace can request a graph placement. Leaving cancels pending raster work and clears both Kitty image IDs without discarding the results or series selection. If a graphics write was already queued, the shell keeps the previous workspace visible until the delete is queued, so the placement is removed before placeholder text is drawn. Returning renders the saved result at the current terminal size without sending a query.

## Verification

- `go test ./...` covers text and shortcut routing, focus and overlay restoration, table and graph state, footer context, resize behavior, and Kitty upload/delete ordering during repeated switches.
- A Rex CLI smoke test against a local Prometheus-compatible fixture checked bare digits, Alt shortcuts, Tab/Shift+Tab focus, and repeated switches after a range query. The query and selected/hidden series survived. The terminal trace showed both Kitty IDs being deleted before the placeholder text and a fresh upload on return. The fixture logged exactly one range request and no Alerts or Targets requests.

## Limitations and next milestone

Narrow headers display numbered tab badges and omit status that does not fit. If a terminal intercepts the Alt shortcuts, leave the editor with Tab and use the bare digits. The Rex smoke test checks terminal output and Kitty packets, not pixel-level artifacts or flicker; for a visual check, run a range query in Rex and look for the image to disappear on Alerts/Targets and return on PromQL as you switch and resize.

In the next milestone, give the first implemented backend view its own request, loading, error, and focus state. Keep unsupported features local to their tab and retain the shell's no-refresh-on-switch behavior for PromQL.
