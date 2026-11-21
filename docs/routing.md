# Exploring routes and receivers

Press **4** to open Routing, then **v** to switch between Routes and Receivers.
Each view keeps its own selection and filter when you switch.

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select a row, or scroll when details have focus |
| `PgUp` / `PgDn`, `Home` / `End` | Move by page or go to the first/last row |
| `←` / `→` or `h` / `l` | Collapse or go to the parent / expand a route |
| `Tab` / `Shift+Tab` | Switch focus between list and details |
| `Enter` in Routes | Inspect the route's effective receiver; `Esc` returns to the same route |
| `Enter` in Receivers | Choose a referencing route, then press `Enter` again to jump to it and expand its ancestors |
| `/` | Fuzzy filter; `Enter` applies, `Esc` cancels. Erase the input and apply to clear |
| `s` in Receivers | Toggle between configuration order and sorting by name |
| `r` | Refresh the configuration |
| `?` | Show Routing help |

Routes are shown in configuration order. A child route is evaluated only when
an alert reaches its parent. All matchers on a route must match. After a sibling
matches, `continue: true` lets Alertmanager evaluate later siblings; the default
is to stop at that match. Details show each route's effective receiver, grouping,
and timing, and mark each setting as explicit, inherited from an ancestor, or a
default. Route filtering searches matcher labels and values and receiver names,
including collapsed branches. A jump clears the Routes filter only if it would
hide the destination.

Receivers lists every receiver, including unused ones. Reference counts include
both explicit and inherited assignments; they are **configuration references**,
not alerts or delivered notifications. Receiver filtering searches names,
integration types, and safe destination metadata.

## Data and privacy

Routing reads `config.original` from **GET `/api/v2/status`** at the current
connection's `alertmanager_url`, using the same TLS settings and timeout as
Silences. It loads on entry when there is no snapshot and refreshes manually;
it does not poll. If a refresh fails, the last successful snapshot for that
connection remains visible and is marked stale. Switching connections discards
that snapshot.

Only allowlisted destination metadata is shown, such as a Slack channel, an
email recipient, or a webhook host. URL credentials, paths, query parameters,
and fragments are omitted. Redacted, templated, or file-backed destinations
may be unavailable. Unknown integrations are listed by type without exposing
their fields. PromTUI does not show or copy the raw configuration, evaluate
templates, send notifications, or modify Alertmanager.

## Example Alertmanager configuration

In this example, `payments` is referenced by both the payments route and its
critical child, which inherits the receiver and overrides `group_wait`.
It has two email integrations. `default` handles the root route.

```yaml
global:
  smtp_smarthost: 'mail.example.org:587'
  smtp_from: 'alerts@example.org'

route:
  receiver: default
  group_by: [alertname]
  group_wait: 30s
  routes:
    - matchers: ['team="payments"']
      receiver: payments
      continue: true
      routes:
        - matchers: ['severity="critical"']
          group_wait: 10s

receivers:
  - name: default
    email_configs:
      - to: 'ops@example.org'
  - name: payments
    email_configs:
      - to: 'payments-oncall@example.org'
      - to: 'payments-leads@example.org'
```
