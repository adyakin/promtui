# PromTUI

A keyboard-driven terminal UI for **Prometheus** and **VictoriaMetrics**. Run PromQL queries, visualize time series, inspect Alertmanager alerts, and browse scrape targets without leaving your terminal.

![PromTUI showing a PromQL query and time-series graph](promtui.png)

## Features

- Multiline PromQL editor with syntax highlighting, completion, history, and saved queries
- Instant-query tables and range-query graphs with series filtering
- Alertmanager alert browser with filtering and sorting
- Alertmanager silence management with matcher previews
- Read-only Alertmanager routing tree and receiver explorer
- Prometheus and vmagent scrape-target inspection
- Multiple backend connections with fast switching

## Quick start

Download the archive for your platform from [GitHub Releases](https://github.com/adyakin/promtui/releases/latest): `darwin_arm64` for Apple Silicon Macs or `linux_amd64` for 64-bit x86 Linux. Extract it, enter the extracted folder, then run:

```sh
cp config.example.yaml config.yaml
./promtui -config config.yaml
```

To build from source instead, use Go 1.25 or newer:

```sh
make build
cp config.example.yaml config.yaml
./bin/promtui -config config.yaml
```

Run `promtui --version` (or `promtui version`) to check which binary you're running, without loading configuration. Use `./bin/promtui --version` to check a local build directly. Release binaries show their release tag; builds between tags identify the commit and indicate whether the working tree had uncommitted changes.

Press **i** to edit a query and **Ctrl+Enter** or **Ctrl+G** to run it. Use **g** for Graph, **t** for Table, **0–4** to switch tabs (**[0] PromQL**, **[1] Alerts**, **[2] Silences**, **[3] Targets**, **[4] Routing**), and **c** to change connections. Use **Alt+0–4** while typing outside modal dialogs. Press **?** for help outside text inputs and **Alt+S** (Option+S on macOS with Option configured as Alt) for request status on PromQL, Alerts, or Targets. PromTUI does not bind function keys. Graphs require a terminal with Kitty graphics support, such as Kitty or Rex.

To investigate an alert, open **[1] Alerts**, select an alert, and press **Enter** to open its details. Press **p** to load the expression from its Prometheus generator link into the PromQL editor. This replaces the current draft without running the query or changing the connection, time range, or results. Review or edit it, then press **Ctrl+Enter** to run it. VictoriaMetrics/vmalert links are not supported.

To silence an alert, press **s** in Alert Details, or open **2 Silences** and press **n** to start from scratch. See [Managing silences](docs/silences.md) for preview, scheduling, editing, and expiration.

To see which receiver a route uses, open **4 Routing** and press **Enter** on a route. Press **v** to browse receivers and jump to routes that reference them. See [Exploring routes and receivers](docs/routing.md).

## Example configuration

```yaml
general:
  default_connection: local

query:
  default_range: 1h

connections:
  local:
    url: http://localhost:9090
    alertmanager_url: http://localhost:9093 # optional
```

Default paths on macOS and Linux:

- **Config:** `~/.config/promtui/config.yaml`, then `config.yml`, then `config.toml`
- **Saved queries:** `~/.local/share/promtui/queries.yaml`
- **History:** `~/.local/share/promtui/history.jsonl`

Start with the full [`config.example.yaml`](config.example.yaml), or pass any YAML or TOML config with `-config PATH`. See the [configuration reference](docs/configuration.md) for TLS options, target sources, and per-connection Alerts and Targets columns and search.

## Publishing a release

Push a `vX.Y.Z` tag on a commit containing the release workflow to GitHub. The workflow runs the tests and publishes both archives and `checksums.txt`. No repository secrets are needed: the release job uses GitHub's built-in token.

```sh
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin refs/tags/v0.1.0
```

Use a remote that pushes to GitHub. If `origin` has multiple push URLs, Git sends the tag to each of them.

To build the release archives locally without publishing, run `goreleaser release --snapshot --clean`.

## License

PromTUI is licensed under the [MIT License](LICENSE.txt).
