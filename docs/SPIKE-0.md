# Spike 0 — Graphics feasibility handoff

**Date:** 2026-11-18
**Status:** Kitty rendering is feasible in Rex. A small Go/Bubble Tea demo renders a live two-series PNG chart and the user visually confirmed that it looks good. **Raw keyboard handling, clearing, and resize handling are not yet verified and have a known Bubble Tea lifecycle defect described below.** Milestone 1 has not been started.

This document is a persistent handoff for an agent with no access to the original conversation. Do not silently upgrade the verification status: reproduce an experiment before claiming it passed.

## 1. Decision

Use the **Kitty graphics protocol** for Rex, with a raster chart encoded as PNG and displayed using classic cursor-based Kitty placements.

Do not spend more time on Sixel for the Rex-specific path unless requirements change. The tested Rex terminal did not advertise Sixel feature 4 in Primary DA, while a valid Kitty graphics query returned `OK`. No Sixel image encoder was implemented or visually tested, so this is a protocol-selection decision, not a claim that a Sixel rendering attempt failed.

The useful architecture is:

1. Generate synthetic or real time-series samples independently of rendering.
2. Rasterize axes, grid, legend, and antialiased lines into a transparent PNG with `github.com/go-analyze/charts/chartdraw`.
3. Give Bubble Tea responsibility for messages, timers, and key decoding, but **not** terminal image rendering.
4. Explicitly manage the alternate screen, cursor, Kitty uploads/placements/deletion, terminal raw mode, and resize notifications.
5. Fit each image with `c=<columns>,r=<rows>` and render its PNG at the pixel geometry reported by `TIOCGWINSZ`.

- Primary DA `ESC[?62;22c` has no feature `4`, so this Rex endpoint does not advertise Sixel.
- The Kitty query uses a direct, 1×1, 24-bit RGB payload (`f=24,s=1,v=1;AAAA`). Rex returned `i=42;OK`.

## 4. What was actually verified

| Experiment | Evidence and status |
| --- | --- |
| Rex Sixel advertisement | Probe returned `ESC[?62;22c`; feature 4 absent. |
| Rex Kitty support | Corrected query returned `ESC_Gi=42;OK ESC\\`. |
| PNG chart appears in Rex | **User visually confirmed the running demo:** “looks amazing actually. I like it.” |
| Live redraw occurs | Rex text captures showed the frame counter advancing (observed values included 6, 87, and 213), and the user saw the live chart. |
| Chart aesthetics | User explicitly liked the result. This is subjective visual confirmation, not a pixel-golden test. |
| Clearing | Explicit `d=A` / `d=I` sequences are implemented, but the interactive clear action was **not verified**. |
| Resizing | **Not working in the current build.** A Rex size change was observed at the terminal, but the model continued to display its original size. See section 7. |
| Remote SSH rendering | Works as intended, with same limitations in demo as local run (no resize/other) |
| Flicker, high refresh rates, memory growth | **Not measured.** Do not claim production readiness from the 1 Hz demo. |

Environment used for the verified local run:

- macOS Darwin/arm64, Go `go1.27.4 darwin/arm64`
- `TERM=xterm-256color`, `TERM_PROGRAM=rex`, `COLORTERM=truecolor`
- Rex app reported `0.1.0 (2033)`; terminal environment reported preview 21

Screenshot capture is not a reliable verification route in this environment. The user asked not to analyze screenshots and believes the app lacks the required permission. Use direct user inspection or protocol-level integration tests instead.

## 5. Kitty implementation details to preserve


- Uses the alternate screen and hides the cursor explicitly.
- Encodes PNG using direct transmission: `a=T,f=100,t=d`.
- Splits base64 into **4096-byte chunks**, using `m=1` for continuation and `m=0` for the final chunk. Do not replace this with one unbounded APC payload.
- Uses image IDs 1 and 2 as a small double buffer: display the new image, then delete the previous ID. This avoids intentionally blanking the graph before every normal redraw.
- Uses placement `p=1`, cell geometry `c=<columns>,r=<rows>`, and `C=1` so the cursor remains controlled by the renderer.
- Positions the image by moving the cursor to graph row 3. `X`/`Y` are pixel offsets within a placement, not a general replacement for layout.
- Suppresses routine responses with `q=2`; the separate `-probe` command is the capability/acknowledgement path.
- Deletes an old image and its data with `a=d,d=I,i=<id>`.
- Uses `a=d,d=A` before a full clear and during cleanup. Do not assume `ESC[2J` alone removes graphics.
- Writes one complete redraw buffer to the terminal instead of interleaving arbitrary log writes with graphics chunks.

The chart area is `terminal rows - 4`; two rows are reserved above it and two below it. The PNG dimensions are:

```text
width  = columns   × cell_width_px
height = graphrows × cell_height_px
```

`TIOCGWINSZ` is preferred over guessed cell sizes. If pixel geometry is zero, the current fallback is 8×16. In the experiment, an unattached Rex tab initially used that fallback; after client attachment Rex reported real geometry such as 22×43 pixels per cell. Production code should treat fallback geometry as degraded and refresh it on every resize/attachment event.

## 6. `rs/jplot` reference findings

Reference source was cloned and studied at:

```text
https://github.com/rs/jplot
commit 2a54bccf26d9cb644ec987994123cf20683fa75b
2025-10-20T14:38:12+01:00
Switch from wcharczuk/go-chart to go-analyze/charts (#32)
```

Relevant upstream files:

- `graph/graph.go`: `chartdraw` series, SI labels, transparent chart setup, flat-range workaround.
- `graph/dash.go`: renders chart(s) to PNG.
- `term/kitty.go`: direct PNG/base64 transmission and alternating image IDs.
- `term/sixel.go`: Primary DA feature-4 detection and `go-sixel` encoding.
- `term/common.go`: cell/window sizing and writer selection.
- `main.go`: cursor save/restore around periodic rendering.

What was borrowed conceptually: PNG rasterization with `chartdraw`, terminal graphics writer boundary, and alternating image IDs.

What was intentionally changed:

- Added protocol-required 4096-byte base64 chunking; upstream Kitty writer emits one payload.
- Added explicit `c`/`r` cell geometry and `C=1` cursor preservation.
- Added explicit old-image and all-image deletion for full clear/cleanup.
- Replaced the invalid dimensionless Kitty query with a valid 1×1 RGB query.
- Kept chart/data generation local and deterministic for Spike 0.

## 7. Known defect: `WithoutRenderer` skips terminal initialization

This is the most important issue for the next agent.

The demo uses:

```go
tea.NewProgram(model, tea.WithoutRenderer())
```

This successfully keeps Kitty APC payloads out of Bubble Tea's line diff renderer, but in Bubble Tea v1.3.10 it also has non-obvious lifecycle effects:

- `options.go`: `WithoutRenderer` installs `nilRenderer`.
- `tty.go:25-29`: `initTerminal` returns immediately for `nilRenderer`.
- As a result, `tty_unix.go:15-29` `initInput` is not called: no raw mode and no `ttyOutput` initialization.
- `tea.go` resize handling only produces `WindowSizeMsg` when `ttyOutput != nil`.

Observed result: Rex reported new dimensions (for example 100×26 cells, 22×43 px/cell), while the model's status remained at its original 80×24, 8×16 px/cell. Therefore resize handling is currently a confirmed defect, not an untested assumption. Normal terminal canonical input also means intended single-key controls cannot be considered verified.

### Recommended fix

Keep explicit graphics rendering, but own the missing terminal lifecycle in `cmd/spike0` or a small terminal-controller package:

1. Call `term.MakeRaw` on stdin before `Program.Run`; restore it on every exit path.
2. Install a `SIGWINCH` listener on Darwin/Linux.
3. Initially and on every `SIGWINCH`, read `TIOCGWINSZ` and send a `tea.WindowSizeMsg` with `program.Send`.
4. Ensure listener shutdown and raw-mode restoration are joined with renderer cleanup.
5. Test `c`, Space, `r`, and `q` as single keystrokes, then repeat clear/show and several pane resizes while watching for stale placements.

An alternative is upgrading to a Bubble Tea version with a supported custom-renderer interface, but that was **not investigated** and should not be assumed simpler. Replacing `WithoutRenderer` with the standard renderer without isolating APC output is not recommended.

## 8. Rex limitations relevant to production

Rex changelog results explicitly advertise full Kitty graphics support, including remote graphics. Community feedback nevertheless reports issues that matter to a live charting application:

- Kitty graphics frame-rate/lag reports remain active as of 2026-11-18. A 1 Hz chart is not evidence that animation or 10–60 Hz updates will be acceptable.
- Some reports concern large images, animation, scrolling, high CPU, and memory/snapshot growth.
- A clear-terminal report says text can clear while a Kitty placement remains; explicit `a=d,d=A` removed it. The demo therefore deletes graphics explicitly.
- Rex text and VT capture did not include the rendered image, so terminal capture alone cannot validate graphics.

Recommended constraints for the next iteration:

- Start at 1 Hz and benchmark before increasing refresh rate.
- Keep image dimensions bounded by the actual chart viewport, not the entire scrollback.
- Avoid animation and Unicode-placeholder virtualization until classic placements are solid.
- Delete superseded images and measure memory during a long run.
- Do not write logs to stdout while graphic chunks are being emitted; use stderr or a log file.

`tmp/` is intentionally ignored by Git so the local artifact is not committed. The SSH probe and UI run remain **untested**. After running them, update this section with the remote probe output and visual/resize results. Rebuilding will normally change the checksum; replace the value above after every rebuild.


