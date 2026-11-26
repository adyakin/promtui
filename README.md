# PromTUI

A keyboard-driven terminal UI for **Prometheus**, **VictoriaMetrics**, and **Alertmanager**.

![PromTUI showing a PromQL query and time-series graph](promtui.png)

## Quick start

Download the archive for your platform from [GitHub Releases](https://github.com/adyakin/promtui/releases/latest): `darwin_arm64` for Apple Silicon Macs or `linux_amd64` for 64-bit x86 Linux. Extract it, enter the extracted folder, then run:

```sh
cp examples/config.example.yaml config.yaml
./promtui -config config.yaml
```

Older release archives put `config.example.yaml` at the archive root; use that path if there is no `examples/` directory.

To build from source instead, use Go 1.26.11 or newer:

```sh
make build
cp examples/config.example.yaml config.yaml
./bin/promtui -config config.yaml
```

Run `promtui --version` (or `promtui version`) to check which binary you're running, without loading configuration. Use `./bin/promtui --version` to check a local build directly. Release binaries show their release tag; builds between tags identify the commit and indicate whether the working tree had uncommitted changes.

Try `up`: press **i**, type the query, and press **Ctrl+Enter** (or **Ctrl+G**). From results, press **g** to run a range graph or **t** to run an instant table. Graphs require a Kitty-compatible graphics terminal such as Kitty or Rex.

## Features by pane

| Tab | What you can do |
| --- | --- |
| **0 PromQL** | Compose multiline queries with syntax highlighting, completion, local diagnostics, history, and saved templates. Run PromQL or backend-supported MetricsQL; local parser diagnostics don't block execution. Browse, sort, and filter result tables; graph time ranges, inspect samples, and hide or isolate series. Inspect or copy labels and query errors. |
| **1 Alerts** | Search and sort Alertmanager alerts, including by label and annotation. Save and reuse label filters, switch named table views, and inspect silenced or inhibited alerts and their [inhibition relationships](docs/03-alerts.md#inhibition). Open the generating rule, load the original expression, graph the alert around activation, or start a silence from its labels. |
| **2 Silences** | [Browse, preview, create, edit, and expire silences](docs/silences.md). Inspect matching alerts and schedule a start and end. Status changes locally as silences start or expire, even without polling. |
| **3 Targets** | Browse active and dropped targets from Prometheus or multiple [vmagent sources](docs/configuration.md#scrape-target-sources). Filter by source, health, or search; customize columns; inspect scrape URLs, labels, timings, and errors. |
| **4 Routing** | Explore the read-only [Alertmanager routing tree and receivers](docs/routing.md). Follow receiver references back to their routes and inspect or copy the selected receiver's YAML. |
| **5 Rules** | Explore alerting and recording rules, groups, evaluations, errors, and alert instances from Prometheus or multiple [vmalert sources](docs/rules.md). Filter by rule type or state, or load a rule's expression into PromQL without running it. |

Switch connections without restarting. Optional [mouse support](docs/configuration.md#mouse-and-terminal-links) enables clicks, scrolling, and graph hover values without rerunning the query. Alert and target URLs are terminal hyperlinks and can be copied from details. [Terminal program status](docs/configuration.md) reports explicit PromQL runs to compatible terminals.

## Keyboard shortcuts

See the [complete keybinding tables by pane and mode](docs/keybindings.md), including search, details, saved-item editors, and confirmations. In-app footers show shortcuts for the current context. PromTUI does not bind function keys.

## Configuration

Start with the [YAML example](examples/config.example.yaml) or the [TOML example](examples/config.example.toml). The default connection points to `http://localhost:9090`. To enable Alerts, Silences, and Routing, set the connection's `alertmanager_url` (for example `http://localhost:9093`). For VictoriaMetrics, use a Prometheus-compatible query URL and configure `vmagent` and `vmalert` separately if the query endpoint doesn't serve targets or rules. The [configuration reference](docs/configuration.md) covers URL prefixes, TLS, custom columns, and format support.

Data panes load on first entry but don't poll by default. Press **r** in the pane's main view to refresh. To opt in for just one pane, add this to YAML:

```yaml
auto_refresh:
  alerts: 30s
```

In TOML, put `alerts = "30s"` under `[auto_refresh]`. Omitted intervals or `0s` are off; only the visible pane polls.

Default paths on macOS and Linux:

- **Config:** `~/.config/promtui/config.yaml`, then `config.yml`, then `config.toml`
- **Saved queries:** `~/.config/promtui/queries.yaml` (falls back to an existing `~/.local/share/promtui/queries.yaml`)
- **Saved alert filters:** `~/.config/promtui/alerts.yaml`
- **Extra Alerts layouts:** `alert_views.d/*.yaml` beside the active config
- **History:** `~/.local/share/promtui/history.jsonl`

Use `-config` for an alternate main config, `-connection` to choose a starting connection, and `-history` to override history storage. For example, start in range mode with `promtui -connection local -range 1h -step 30s`. `-end` pins evaluation time; an explicit `-start` requires a later `-end` instead of `-range` (both timestamps use RFC3339). Use `-graphics off` for a text-only terminal or `-graphics kitty` to force Kitty graphics.

## Alert workflows

### Graph an alert or save its expression

In Alerts, select an alert and open details with **Enter**. Press **p** to run a historical graph ending now and reaching back to roughly an hour before activation, up to seven days. PromTUI can remove a simple outer threshold and scope the expression to the alert instance when it can verify the match; check the query notice to see what changed. To save the original expression instead, press uppercase **P**, then **Ctrl+S** in the editor. Press **r** in Alert Details to find the generating rule. PromTUI does not create or edit server-side alerting rules.

### Create a named Alerts view

Add this at the top level of `config.yaml` (merge it into any existing `alerts.views` mapping):

```yaml
alerts:
  views:
    compact:
      columns:
        - {source: labels.alertname, title: Alert}
        - {source: labels.severity, title: Severity, width: 10}
```

Restart PromTUI, open Alerts, and choose `compact` with **Ctrl+O**. A view changes columns without changing your filters. Set `alerts.default_view: compact` under a YAML connection to make it the starting layout. See [named views](docs/configuration.md#named-alerts-views) for weights, annotation extraction, and standalone view files.

## Saved PromQL queries and alert filters

Create the YAML files in `~/.config/promtui/` (create the directory with `mkdir -p ~/.config/promtui`). On a fresh setup, you can copy [`examples/queries.yaml`](examples/queries.yaml) and [`examples/alerts.yaml`](examples/alerts.yaml) there as-is. If you already have a file, append entries under its existing `queries:` or `alerts:` list instead of overwriting it. Names must be unique within each file, ignoring case.

Saved items are shared across connections and do not move with `-config`. If you still use `~/.local/share/promtui/queries.yaml`, add entries there unless you move that file to `~/.config/promtui/`.

### PromQL: `queries.yaml`

```yaml
queries:
  - name: All scrape targets
    expression: up

  - name: Scrape targets by job
    parameters:
      - name: job
    expression: 'up{job="${job}"}'
```

`up` is generated by Prometheus for every scrape target: `1` means the last scrape succeeded, and `0` means it failed. For the second query, use a `job` label from the results of `up`, such as `prometheus` if Prometheus scrapes itself. Loading fills the editor without executing the query.

### Alerts: `alerts.yaml`

To save a local alert query in the app, press **m** in Alerts, enter a matcher such as `severity = critical`, apply with **Ctrl+Enter**, then use **Ctrl+S** to name and save it. Reopen it with **Ctrl+O**. This filters the current Alertmanager results; it does not change alerts on the server.

```yaml
alerts:
  - name: All named alerts
    matchers:
      - label: alertname
        operator: "=~"
        value: ".+"

  - name: Alerts by severity
    parameters:
      - name: severity
        default: warning
    matchers:
      - label: severity
        operator: "="
        value: "${severity}"
```

Filters match alerts already returned by the configured Alertmanager. `alertname` is standard; `severity` and its values depend on your rules, so change `warning` to a value your alerts use. Matchers support `=`, `!=`, `=~`, and `!~`; regexes match the whole label value, and all matchers in a filter must match.

Omit `parameters` for a plain saved item. In a template, declare each `${name}` once under `parameters`. A missing `default` means PromTUI asks for a value when loading; a quoted `default: ""` suggests an empty value. Substitution is literal string insertion, with no automatic PromQL or regex escaping. Use `$${name}` for a literal `${name}` in template text; a plain PromQL query without `parameters` already treats dollar signs literally.

## Publishing a release

Once the manual workflow is on the default branch, push a `vX.Y.Z` tag for the commit you want to release. Tag a commit that includes the manual workflow: tagging an older commit that still has the tag-push trigger can start a release automatically.

```sh
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin refs/tags/v0.1.0
```

Use a remote that pushes to GitHub. If `origin` has multiple push URLs, Git sends the tag to each of them.

With the GitHub CLI installed and authenticated, start the release workflow for the pushed tag:

```sh
make release TAG=v0.1.0
```

You can also use **Actions → Release → Run workflow** in GitHub and enter the tag. The workflow checks out that tag, runs the tests, and publishes both archives and `checksums.txt` using GitHub's built-in token.

With GoReleaser installed, run `make release-local` to build snapshot archives and `checksums.txt` in `dist/` without publishing. This replaces any previous contents of `dist/`.

## License

PromTUI is licensed under the [MIT License](LICENSE.txt).
