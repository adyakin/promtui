# PromTUI patch guide

- **Storage:** Keep user-editable config and saved queries/alert filters in `~/.config/promtui/`; keep history and other app-managed persistent data in `~/.local/share/promtui/` on macOS and Linux. Preserve `-config` and existing file fallbacks. See [configuration](docs/configuration.md) and [saved items](docs/saved-items.md).
- **Keys:** Do not bind function keys. Follow existing shortcuts, avoid intercepting text input, and update contextual help when bindings change.
- **Dialogs:** Use floating overlays, not full-screen replacements or layout reflow. Overlays capture focus, preserve the underlying layout and state (including Kitty graphs), and restore focus on close. See [overlay requirements](docs/uix-layover.md).
- **Backends:** Keep both Prometheus and VictoriaMetrics working. Preserve URL path prefixes and account for separate `vmagent`, `vmalert`, and Alertmanager endpoints where relevant. Don't block MetricsQL queries on local PromQL parser warnings. Cover both backends when changing shared API behavior.

## Go checks

- Use the Go version in `go.mod` or newer. Keep patches focused, follow existing package boundaries, and prefer the standard library over new dependencies.
- Keep network I/O out of rendering and synchronous UI updates. Propagate request contexts and return errors with useful context.
- Add focused regression tests for behavior changes. Use `httptest` or fixtures for API tests so they don't require live backends.
- Run `gofmt` on changed Go files, `go vet ./...`, and `make test` (`go test ./...`). Run the repo's linter if one is configured and `go test -race ./...` for concurrency changes.
