# PromTUI — Graph Mouse Inspection and Readable Series Names

Implement a small, focused improvement to the existing PromQL graph UI.

## 1. Mouse inspection

When the mouse moves over the graph's plotting area:

- Map the mouse's terminal-cell X coordinate to the nearest timestamp.
- Show that timestamp and the corresponding values for visible series.
- Use existing query result samples; do not execute additional PromQL requests.
- Display the information in a compact text panel below the graph.
- Show the panel only while inspecting the graph.
- Limit its height when many series are present.
- Preserve series colors in the inspection panel.
- Clear inspection when the mouse leaves the graph.

Use the nearest available sample for each series. Clearly handle missing samples rather than presenting stale or fabricated values.

**Performance:** Mouse movement must not regenerate or retransmit the Kitty graph image. Update only the lightweight inspection UI.

## 2. Readable series names

Replace full Prometheus label sets in the graph legend and series selector with compact names.

Prefer:

1. `instance`
2. `servicename`
3. `job`
4. A compact fallback derived from available labels

When multiple series have identical names, append distinguishing labels such as `mountpoint`, `device`, or another differing label.

For example:

```text
pgsql01:9100 · /
pgsql01:9100 · /var/lib/postgresql
pgsql02:9100 · /
```

Ensure displayed names remain unique within the current query results. Preserve full labels in the existing series details.

Use the same display-name logic in the legend, selector, and inspection panel.

## 3. Compatibility and testing

- Reuse existing graph rendering and mouse infrastructure.
- Preserve keyboard navigation and series visibility controls.
- Respect `ui.mouse` configuration.
- Handle terminal resizing and graph scrolling.
- Test timestamp mapping, missing samples, duplicate series names, hidden series, and mouse-leave behavior.
- Run `go test ./...` and existing build checks.
- Perform Rex smoke tests where the environment permits.

## Out of scope

No graph redesign, zooming, panning, floating tooltips, axis changes, new configuration options, or Alert → Rule navigation.

Keep the implementation small and avoid unrelated refactoring.
