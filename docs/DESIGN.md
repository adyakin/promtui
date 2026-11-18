Read `docs/DESIGN.md` and implement **Spike 0 — Graphics feasibility**.

Focus exclusively on Rex terminal graphics.

Investigate Sixel/DRCS support, study `rs/jplot` as a rendering reference, and prototype high-quality time-series graphs in Go.

Test image placement, redraws, clearing and resizing, including integration with a minimal Charm Bubble Tea application.

If Sixel is unsuitable, investigate Kitty graphics protocol instead.

Do not implement PromQL queries, autocomplete or the main application yet.

Keep the prototype small and runnable. Document findings, limitations and the recommended approach in `docs/SPIKE-0.md`.

Do not proceed to Milestone 1 automatically.
