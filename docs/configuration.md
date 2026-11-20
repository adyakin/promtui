# Configuration

YAML is the preferred format. Without `-config`, PromTUI checks the `promtui`
subdirectory of Go's [user config directory](https://pkg.go.dev/os#UserConfigDir)
for `config.yaml`, `config.yml`, then `config.toml`. The first file found wins;
if it cannot be read or parsed, PromTUI reports the error without trying a lower
priority file. If none exists, PromTUI uses the built-in defaults.

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
| `connections` | One connection, `local`, at `http://localhost:9090` |
| `insecure_skip_verify` | `false` |

Durations use Go syntax (for example, `168h` for seven days). Every connection
requires a Prometheus or VictoriaMetrics `url` and can have an optional
`alertmanager_url`. Both are HTTP(S) base URLs without credentials, query
strings, or fragments. Path prefixes are supported.

## TLS verification

HTTPS certificates are verified by default. In either format, the top-level
`insecure_skip_verify` applies to all connections unless the connection sets its
own value. An explicit `false` overrides a global `true`, just as a connection's
`true` overrides a global `false`. The effective value applies to PromQL queries,
editor suggestions, and Alertmanager for that connection only.

Use `true` only when you knowingly accept the risk of an unverified endpoint:
it allows impersonation of the HTTPS server.
