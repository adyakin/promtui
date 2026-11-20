# PromTUI

A keyboard-driven terminal UI for **Prometheus** and **VictoriaMetrics**. Run PromQL queries, visualize time series, inspect Alertmanager alerts, and browse scrape targets without leaving your terminal.

![PromTUI showing a PromQL query and time-series graph](promtui.png)

## Features

- Multiline PromQL editor with syntax highlighting, completion, history, and saved queries
- Instant-query tables and range-query graphs with series filtering
- Alertmanager alert browser with filtering and sorting
- Prometheus and vmagent scrape-target inspection
- Multiple backend connections with fast switching

## Quick start

Download the archive for your platform from [GitHub Releases](https://github.com/adyakin/promtui/releases/latest): `darwin_arm64` for Apple Silicon Macs or `linux_amd64` for 64-bit x86 Linux. Extract it, then run:

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

Press **i** to edit a query and **Ctrl+Enter** to run it. Use **g** for Graph, **t** for Table, **0–2** to switch tabs, **c** to change connections, and **?** for help. Graphs require a terminal with Kitty graphics support, such as Kitty or Rex.

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

Start with the full [`config.example.yaml`](config.example.yaml), or pass any YAML or TOML config with `-config PATH`. See the [configuration reference](docs/configuration.md) for all settings, TLS options, and target sources.

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
