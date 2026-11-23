# Configuration

YAML is the preferred format. Without `-config`, PromTUI checks
`~/.config/promtui` on macOS and Linux for `config.yaml`, `config.yml`, then
`config.toml`. The first file found wins; if it cannot be read or parsed,
PromTUI reports the error without trying a lower priority file. If none exists,
PromTUI uses the built-in defaults.

An explicit `-config PATH` selects the main configuration file, choosing the parser by
extension. Existing TOML configs need no conversion. To switch to YAML, copy
[`config.example.yaml`](../examples/config.example.yaml) and turn each TOML
`[[connections]]` entry into a YAML key under `connections`, using its `name`
as the key. A [TOML example](../examples/config.example.toml) is also available.
PromTUI never rewrites either file.

| Setting | Default |
| --- | --- |
| `general.default_connection` | First connection in file order; use `-connection NAME` to override at startup |
| `general.history_limit` | `1000`; `0` disables history |
| `query.timeout` | `30s` |
| `query.default_range` | `1h` |
| `query.max_series` | `500` |
| `terminal.program_status` | `true`; reports explicit PromQL query status to the terminal via OSC 7501 |
| `ui.mouse` | `false`; enables optional mouse reporting |
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

## Mouse and terminal links

Set `ui.mouse: true` to click tabs and select visible rows, double-click to
open supported row details, and use the wheel in scrollable panes and dialogs.
Hover over a range graph to inspect the timestamp and visible series values
without running another query.
Mouse reporting is optional and works over SSH when the terminal forwards
mouse events. Set `ui.mouse: false` and restart PromTUI to restore uncaptured
terminal mouse behavior. Many terminals also let you hold Shift to select
text while mouse reporting is on.

Target and alert details show HTTP(S) URLs as terminal hyperlinks (OSC 8).
Use your terminal's usual modifier-click to open them. Press `u` in details
to copy the scrape or generator URL; with mouse reporting on, a plain left
click on any visible part of a link copies that URL. Clipboard writes go
to your local terminal over OSC 52, including over SSH. If the terminal or
multiplexer blocks OSC 52, allow it in its settings or select and copy the
text manually. PromTUI does not open a browser or check whether links are
reachable.

## Scrape target sources

The Targets tab uses the connection `url` by default. If that URL only serves
queries, such as a VictoriaMetrics `vmselect` endpoint, set `vmagent` on the
YAML connection to a nonempty list of named Prometheus or vmagent base URLs:

```yaml
connections:
  cluster:
    url: http://vmselect:8481/select/0/prometheus
    vmagent:
      - {name: east, url: "https://monitoring.example.org/vmagent-east"}
      - {name: west, url: "https://monitoring.example.org/vmagent-west"}
```

Names must be unique within a connection. Each source uses the same HTTP(S)
base URL rules, including support for reverse-proxy path prefixes. Configuring
`vmagent` replaces the fallback, rather than adding to it. The old YAML key
`targets` is still accepted as an alias, but do not set both on the same
connection. Existing TOML configurations continue to use the connection URL
for Targets.

## Rule sources

Rules uses the connection `url` unless a nonempty `vmalert` list is configured
on the YAML connection. That list replaces the fallback. Each entry needs a
unique name and an HTTP(S) base URL without credentials, query or fragment.
Sources inherit the connection's TLS verification setting and the configured
request timeout. See [Rules Explorer](rules.md) for an example.

## Alerts and Targets columns and search

In YAML, each connection can set `alerts.columns`, `alerts.search`,
`targets_view.columns`, and `targets_view.search`. `targets_view` only controls
the table and filter; `vmagent` defines where to fetch targets.

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
    vmagent:
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

### Named Alerts views

Define shared layouts under the top-level YAML `alerts.views` mapping, and
optionally select a starting view with `alerts.default_view` on each connection:

```yaml
alerts:
  views:
    certificates:
      columns:
        - {source: labels.severity, width: 10}
        - {source: labels.alertname, weight: 2}
        - {source: labels.path, title: Certificate, weight: 3}
        - source: annotations.summary
          title: Expires in
          extract: 'expires in (.+?)\.?$'
          weight: 2

connections:
  local:
    url: http://localhost:9090
    alertmanager_url: http://localhost:9093
    alerts:
      default_view: certificates
```

Each `source` is a built-in Alerts field, `labels.<name>`, or
`annotations.<name>`. `width` sets a preferred width in terminal cells;
`weight` gives flexible columns a relative share of space. Use one or neither.
The table sizes unspecified columns automatically and can truncate values on
narrow terminals. `extract` takes a Go regular expression with exactly one
capture group; a missing value or non-match displays `—`. Views change only
the displayed columns, not the connection's `alerts.search` fields. Sorting
an extracted column uses the raw source value, not the displayed capture.
On narrow terminals, `…` at the end of the heading row means columns were
omitted.

You can also put `*.yaml` files under `alert_views.d/` next to the active main
config (normally `~/.config/promtui/alert_views.d/`). Each external file starts
with `views:` instead of `alerts:`, followed by the same mapping of names to
layouts. Files are loaded alphabetically. Duplicate names, including names
from the main config, are rejected rather than merged. This is separate from
`alerts.yaml`, which stores saved label filters.

Use **Ctrl+O** in Alerts to open a filter or a view. View selections are
remembered separately for each connection until exit. Press uppercase **D**
to restore the connection's configured default, or its legacy columns (or the
built-in layout) if no default is configured.

## TLS verification

HTTPS certificates are verified by default. In either format, the top-level
`insecure_skip_verify` applies to all connections unless the connection sets its
own value. An explicit `false` overrides a global `true`, just as a connection's
`true` overrides a global `false`. The effective value applies to PromQL queries,
editor suggestions, Alertmanager, and every target source for that connection.
Target sources do not have their own TLS overrides.

Use `true` only when you knowingly accept the risk of an unverified endpoint:
it allows impersonation of the HTTPS server.
