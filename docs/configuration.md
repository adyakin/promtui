# Configuration

YAML is the preferred format. Without `-config`, PromTUI checks
`~/.config/promtui` on macOS and Linux for `config.yaml`, `config.yml`, then
`config.toml`. The first file found wins; if it cannot be read or parsed,
PromTUI reports the error without trying a lower priority file. If none exists,
PromTUI uses the built-in defaults.

An explicit `-config PATH` loads only that file, choosing the parser by
extension. Existing TOML configs need no conversion. To switch to YAML, copy
[`config.example.yaml`](../config.example.yaml) and turn each TOML
`[[connections]]` entry into a YAML key under `connections`, using its `name`
as the key. A [TOML example](../config.example.toml) is also available.
PromTUI never rewrites either file.

| Setting | Default |
| --- | --- |
| `general.default_connection` | First connection in file order; use `-connection NAME` to override at startup |
| `general.history_limit` | `1000`; `0` disables history |
| `query.timeout` | `30s` |
| `query.default_range` | `1h` |
| `query.max_series` | `500` |
| `terminal.program_status` | `true`; reports explicit PromQL query status to the terminal via OSC 7501 |
| `silence_creator` | Empty; optional editable default for the Created by field in new silence drafts |
| `connections` | One connection, `local`, at `http://localhost:9090` |
| `insecure_skip_verify` | `false` |

Durations use Go syntax (for example, `168h` for seven days). Every connection
requires a Prometheus or VictoriaMetrics `url` and can have an optional
`alertmanager_url` for Alerts, [Silences](silences.md), and [Routing](routing.md). Both are HTTP(S)
base URLs without credentials, query strings, or fragments. Path prefixes
are supported.

Set `terminal.program_status` to `false` to suppress OSC 7501 reports. The
setting does not change the in-app status or error messages.

## Scrape target sources

The Targets tab uses the connection `url` by default. If that URL only serves
queries, such as a VictoriaMetrics `vmselect` endpoint, set `targets` on the
YAML connection to a nonempty list of named Prometheus or vmagent base URLs:

```yaml
connections:
  cluster:
    url: http://vmselect:8481/select/0/prometheus
    targets:
      - {name: east, url: "https://monitoring.example.org/vmagent-east"}
      - {name: west, url: "https://monitoring.example.org/vmagent-west"}
```

Names must be unique within a connection. Each source uses the same HTTP(S)
base URL rules, including support for reverse-proxy path prefixes. Configuring
`targets` replaces the fallback, rather than adding to it. Existing TOML
configurations continue to use the connection URL for Targets.

## Alerts and Targets columns and search

In YAML, each connection can set `alerts.columns`, `alerts.search`,
`targets_view.columns`, and `targets_view.search`. `targets_view` only controls
the table and filter; `targets` still defines where to fetch targets.

Each list replaces its own default. Set only `columns` to change what's visible
without changing search, or set only `search` to change `/` without changing
the table. Columns appear in YAML order. A column can be a field name or
`{field: ..., title: ...}` to override its heading. Built-in fields use their
usual headings; `labels.servicename` defaults to `SERVICENAME`.

| View | Built-in fields you can configure |
| --- | --- |
| Alerts | `state`, `severity`, `alertname`, `instance`, `duration` |
| Targets | `health`, `instance`, `job`, `source`, `last_scrape`, `duration`, `scrape_url`, `scrape_pool`, `global_url`, `last_error`, `scrape_interval`, `scrape_timeout` |

Use `labels.<name>` for a label value in either view. Everything after `labels.`
is the literal name, including any dots. For Targets this reads the final `labels`
map, not discovery metadata. An unavailable label displays `—` and contributes
no search text. Labels don't have to exist when PromTUI loads the config.

`/` still matches characters in order, case-insensitively, within a single
field. With a `search` list it only searches those values, including hidden
fields if listed. Without it, Alerts searches state, severity, alert name,
instance, and duration. Targets retains its default search across health,
source, scrape fields, and final and discovered labels. An explicitly
configured Targets `last_scrape` displays the age but searches the timestamp.

Without `columns`, Alerts shows State, Severity, Alert, Instance, and Duration;
Targets shows Health, Instance, Job, Source, Last scrape, and Duration. On
narrow terminals, headings and values may be truncated and rightmost columns
may be clipped. `s` cycles through the existing sort modes, then through any
additional configured columns. Label sorts are ascending, with missing values
last. The default sort on entry is unchanged.

For Prometheus with its built-in Targets endpoint and a separate Alertmanager:

```yaml
connections:
  local:
    url: http://localhost:9090
    alertmanager_url: http://localhost:9093
    alerts:
      columns: [state, {field: labels.alertname, title: Alert}, duration]
    targets_view:
      search: [labels.job, labels.instance, scrape_url]
```

For VictoriaMetrics with two vmagent sources and a final `servicename` label:

```yaml
connections:
  production:
    url: http://vmselect:8481/select/0/prometheus
    alertmanager_url: https://monitoring.example.org/alertmanager
    alerts:
      columns:
        - state
        - {field: labels.severity, title: Severity}
        - {field: labels.servicename, title: Service}
        - {field: labels.alertname, title: Alert}
        - duration
      search: [labels.servicename, labels.alertname, labels.instance]
    targets:
      - {name: east, url: "https://monitoring.example.org/vmagent-east"}
      - {name: west, url: "https://monitoring.example.org/vmagent-west"}
    targets_view:
      columns:
        - health
        - {field: labels.servicename, title: Service}
        - {field: labels.instance, title: Host}
        - last_scrape
      search: [labels.servicename, labels.instance]
```

Omit a list to use its default; empty lists are invalid. Unknown built-ins,
`labels.` without a name, duplicate column fields, and invalid column objects
(missing `field`, blank `title`, or unknown properties) fail at startup.
Errors identify the connection and setting, for example
`connection "production": alerts.columns[2]: unknown field "service"`.

## TLS verification

HTTPS certificates are verified by default. In either format, the top-level
`insecure_skip_verify` applies to all connections unless the connection sets its
own value. An explicit `false` overrides a global `true`, just as a connection's
`true` overrides a global `false`. The effective value applies to PromQL queries,
editor suggestions, Alertmanager, and every target source for that connection.
Target sources do not have their own TLS overrides.

Use `true` only when you knowingly accept the risk of an unverified endpoint:
it allows impersonation of the HTTPS server.
