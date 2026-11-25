# PromTUI patch guide

- **Storage:** Keep user-editable config and saved queries/alert filters in `~/.config/promtui/`; keep history and other app-managed persistent data in `~/.local/share/promtui/` on macOS and Linux. Preserve `-config` and existing file fallbacks. See [configuration](docs/configuration.md) and [saved items](docs/saved-items.md).
- **Keys:** Do not bind function keys. Follow existing shortcuts, avoid intercepting text input, and update contextual help when bindings change.
- **Dialogs:** Use floating overlays, not full-screen replacements or layout reflow. Overlays capture focus, preserve the underlying layout and state (including Kitty graphs), and restore focus on close. See [overlay requirements](docs/uix-layover.md).
- **Backends:** Keep both Prometheus and VictoriaMetrics working. Preserve URL path prefixes and account for separate `vmagent`, `vmalert`, and Alertmanager endpoints where relevant. Don't block MetricsQL queries on local PromQL parser warnings. Cover both backends when changing shared API behavior.

## Development

- Use feature branches and Git worktrees for new features.
- Create worktrees under a dedicated private temporary directory using `mktemp -d`. Prefer `$TMPDIR` when available; otherwise use `/tmp`. Do not create worktrees inside the main repository or alongside it.
- Use separate worktrees for parallel agents to avoid conflicting changes.
- Do not modify unrelated files or overwrite existing uncommitted changes.
- Remove temporary worktrees after successful integration using `git worktree remove`. Do not remove worktrees containing uncommitted changes.
- Merge commit messages should include a meaningful summary of all new features, changes, and relevant fixes.
- Complete the required Go and security checks before committing and pushing changes to the remote repository.

## Go checks

- Use the Go version in `go.mod` or newer. Keep patches focused, follow existing package boundaries, and prefer the standard library over new dependencies.
- Keep network I/O out of rendering and synchronous UI updates. Propagate request contexts and return errors with useful context.
- Add focused regression tests for behavior changes. Use `httptest` or fixtures for API tests so they don't require live backends.
- Run `gofmt` on changed Go files, `go vet ./...`, and `make test` (`go test ./...`). Run the repo's linter if one is configured and `go test -race ./...` for concurrency changes.

## Security checks

- Scan the repository for known dependency vulnerabilities using both Trivy and Grype before committing and pushing changes.
- Run the scans from the feature worktree against its current dependency state, including `go.mod` and `go.sum`.

```bash
trivy fs --scanners vuln .
grype dir:.
```

- Ensure vulnerability databases are up to date. Do not treat failed scans or unavailable vulnerability databases as successful checks.
- Review findings from both scanners, including vulnerabilities in direct and transitive dependencies.
- If a vulnerable Go dependency has a compatible fixed version, update it before committing.
- Prefer targeted dependency updates over indiscriminate upgrades of all modules.
- Run `go mod tidy` after dependency changes and review the resulting `go.mod` and `go.sum` diff.
- Repeat Go checks and vulnerability scans after updating dependencies.
- Do not suppress, ignore, or exclude vulnerabilities merely to obtain clean scan results.
- If a vulnerability has no available fix, requires a breaking dependency upgrade, or cannot be safely resolved within the current task, report it explicitly with the affected dependency, vulnerability ID, severity, and available remediation options.
- If unresolved vulnerabilities or failed security checks remain, request approval before committing or pushing.
- Perform final security checks on the integrated branch before pushing it to the remote repository.
- Include dependency updates and unresolved security findings, if any, in the final implementation report.

