# PromTUI

A keyboard-driven terminal UI for **Prometheus** and **VictoriaMetrics**. Run PromQL queries, visualize time series, inspect Alertmanager alerts, and browse scrape targets without leaving your terminal.

![PromTUI showing a PromQL query and time-series graph](promtui.png)

## Features

- Multiline PromQL editor with syntax highlighting, completion, history, and parameterized saved queries
- Instant-query tables and range-query graphs with series filtering
- Alertmanager alert browser with sorting, fuzzy search, and reusable label filters
- Alertmanager silence management with matcher previews
- Read-only Alertmanager routing tree and receiver explorer
- Prometheus and vmagent scrape-target inspection
- Multiple backend connections with fast switching

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

Press **i** to edit a query and **Ctrl+Enter** or **Ctrl+G** to run it. Use **g** for Graph, **t** for Table, **0–5** to switch tabs (**[0] PromQL**, **[1] Alerts**, **[2] Silences**, **[3] Targets**, **[4] Routing**, **[5] Rules**), and **c** to change connections. Use **Alt+0–5** while typing outside modal dialogs. Press **?** for help outside text inputs and **Alt+S** (Option+S on macOS with Option configured as Alt) for request status on PromQL, Alerts, or Targets. PromTUI does not bind function keys. Graphs require a terminal with Kitty graphics support, such as Kitty or Rex.

To investigate an alert, open **[1] Alerts**, select an alert, and press **Enter**. Press **p** to run a historical graph on the current connection. PromTUI removes a simple outer threshold and scopes the graph when it can verify that alert labels identify a result series. Check the query notice to see what changed. Press **P** to open the original expression without running it, or **r** to find the generating rule. This works with Prometheus graph links and [configured vmalert sources](docs/rules.md).

The graph ends now and uses the smallest range preset that includes roughly an hour before activation, up to seven days. You can edit the query with **i** or adjust the range with **r** in PromQL. If scoping is unavailable, the graph may include other alert instances.

To silence an alert, press **s** in Alert Details, or open **2 Silences** and press **n** to start from scratch. See [Managing silences](docs/silences.md) for preview, scheduling, editing, and expiration.

To see which receiver a route uses, open **4 Routing** and press **Enter** on a route. Press **v** to browse receivers and jump to routes that reference them. See [Exploring routes and receivers](docs/routing.md).

Open **5 Rules** to browse alerting and recording rules, inspect active alert instances, and load a rule's expression into PromQL with **p** without running it. See the [Rules Explorer guide](docs/rules.md) for shortcuts and `vmalert` setup.

## Configuration

Start with the [YAML config example](examples/config.example.yaml) or the [TOML config example](examples/config.example.toml). The default connection points to Prometheus at `http://localhost:9090`. To use the Alerts tab, set `alertmanager_url` for that connection, for example `http://localhost:9093`. See the [configuration reference](docs/configuration.md) for more options.

In Alerts, press `Enter` to see [inhibition sources, targets, and matching rules](docs/03-alerts.md#inhibition). When related alerts are available, press `i` to choose one and `Esc` to go back.

Use **Ctrl+O** to open saved filters or [named table views](docs/configuration.md#named-alerts-views), and **D** to reset the view for the current connection. In **/** search, use **↑/↓** to highlight a suggested alert name before pressing **Enter**, or press **Enter** without selecting a suggestion to apply the text you typed.

Default paths on macOS and Linux:

- **Config:** `~/.config/promtui/config.yaml`, then `config.yml`, then `config.toml`
- **Saved queries:** `~/.config/promtui/queries.yaml` (falls back to an existing `~/.local/share/promtui/queries.yaml`)
- **Saved alert filters:** `~/.config/promtui/alerts.yaml`
- **History:** `~/.local/share/promtui/history.jsonl`

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

Push a `vX.Y.Z` tag on a commit containing the release workflow to GitHub. The workflow runs the tests and publishes both archives and `checksums.txt`. No repository secrets are needed: the release job uses GitHub's built-in token.

```sh
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin refs/tags/v0.1.0
```

Use a remote that pushes to GitHub. If `origin` has multiple push URLs, Git sends the tag to each of them.

With GoReleaser installed, run `make release` to build snapshot archives and `checksums.txt` in `dist/` without publishing. This replaces any previous contents of `dist/`.

## License

PromTUI is licensed under the [MIT License](LICENSE.txt).
