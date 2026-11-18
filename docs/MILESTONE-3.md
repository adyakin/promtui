# Milestone 3 — Range graphs

Run `make run` in Rex, switch to range mode with Ctrl+T and execute:

```promql
100 * (1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])))
```

The chart uses the successful query’s requested time domain and step, even if the editor’s time settings are changed afterward. Ctrl+W selects the next window; execute to apply. F4 toggles graph/table without refetching. Esc focuses results; `g` also toggles the view there. Up/down selects the legend entry and Space toggles visibility (`●` / `○`). The first eight series start visible, and all returned series are available in the legend. Instant queries always display as tables.

## Implementation

- `internal/query`: the normalized result carries the actual start, end and step together with timestamped samples. The graph and table share sample storage; table sorting cannot reorder the graph’s series.
- `internal/graph`: terminal-independent PNG rendering with `gg`, the embedded Go font, antialiased colored lines, UTC ticks, grid and a dark background. Scaling uses finite samples of visible series. Flat domains are expanded, isolated samples get dots, and non-finite values or gaps exceeding 1.5 evaluation steps break the line. The legend is terminal text, so names and controls remain accessible independently of the PNG.
- `internal/app/graph.go`: a cancellable Tea command rasterizes and encodes each frame. Geometry comes from the graph viewport and `TIOCGWINSZ`, falling back to 8×16 cells when no pixel size is reported. The raster is capped at 3200×2000. The event loop coalesces geometry/visibility changes and rejects obsolete frames. Opening a popup or help removes the placement; closing it restores the graph. Empty results, hidden series and undersized panes remain usable as text.
- `internal/kitty`: classic direct PNG upload with 4096-byte base64 chunks, explicit row/column placement, cursor save/restore and two alternating image IDs. Bubble Tea v2’s `RawMsg` keeps graphics out of text diffing while retaining normal keyboard and SIGWINCH handling. Cleanup deletes only the two app-owned IDs. Normal quit waits for pending output and history, with a final cleanup after terminal restoration.
- Automatic selection recognizes Rex/Kitty environment hints. Without those hints (common over SSH), it sends a non-displaying Kitty graphics query through the terminal and enables graphs on a matching `OK`. No response leaves table mode available. Use `-graphics kitty` to force graphics or `-graphics off` to disable probing and rendering.

## Verification

The live CPU query above returned six series on `q1q1`. In Rex, keyboard controls selected and hid a series, toggled the table and opened/closed help; direct terminal resizing from 100×30 to 60×18 relaid out the graph view. In a separate protocol-recorded run, the original and visibility-adjusted uploads decoded to distinct 640×272 PNGs; explicit image deletion was recorded. Rex text capture does not show the pixels: these checks verify transport and interaction, not visual approval of the final graph.

`go test -race ./...` covers the actual raster across flat values, missing samples and NaN, no-finite-data handling, Kitty chunk reassembly, superseded resizes, overlays and table switches during an in-flight upload. `go vet ./...` and `make build` check the binary.

Graphs update on query, layout or visibility changes; there is no animation loop. Native histograms are still omitted with a notice. Remote graphics, sustained high-rate redraws and visual quality across different Rex font sizes have not been verified.
