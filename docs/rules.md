# Rules Explorer

Press **5** (**Alt+5** while typing) to open the read-only Rules tab. Browse
sources, groups, rules, and active alert instances in the tree. With one
healthy source, groups are shown directly. The Details panel shows expressions,
labels, annotations, evaluation metadata and errors when the endpoint supplies
them. Alert instance details also include their current value and start time
when available.

| Key | Action |
| --- | --- |
| `↑` / `↓`, `k` / `j` | Select a tree row or scroll Details |
| `←` / `→`, `h` / `l` | Collapse or go to parent; expand or go to first child |
| `Enter` | Toggle a branch or focus a leaf's details |
| `Tab`, `Shift+Tab` | Switch focus between the tree and Details |
| `/` | Fuzzy search; `Enter` applies, `Esc` cancels or clears |
| `f` | Cycle All types, Alerting, Recording |
| `s` | Cycle All states, Firing, Pending, Inactive |
| `p` | Load the selected rule's expression into PromQL without running it |
| `e` | Open a scrollable full error when one is available; `Esc` closes it |
| `r` | Refresh now; `Ctrl+C` cancels an in-progress refresh |
| `?` | Help |

Search checks source, group and rule names, expressions, labels, annotations
and instance labels. Matches reveal their ancestors even when the tree is
collapsed. A specific alerting state excludes recording rules.

Firing and pending are **evaluation** states, not Alertmanager notification
states. Rules does not indicate whether Alertmanager silenced or inhibited an
alert. On an alert instance, `p` loads the parent expression without adding
label matchers. For a recording rule it loads the defining expression, not
the output metric name. Existing query results remain until you run a query.
The local Prometheus parser may warn about MetricsQL syntax; warnings do not
block execution. Expressions beyond the editor's capacity stay in Details
rather than being silently truncated.

## Choose rule sources

Standalone Prometheus needs no extra configuration: Rules requests
`/api/v1/rules` from the connection's primary `url`. To use vmalert, add a
nonempty list of named sources to the YAML connection:

```yaml
connections:
  production:
    url: http://vmselect:8481/select/0/prometheus
    vmalert:
      - name: east
        url: https://monitoring.example.org/vmalert-east
      - name: west
        url: https://monitoring.example.org/vmalert-west
```

For that connection, only the listed sources are queried. Prometheus and
vmalert use the same endpoint, with path prefixes preserved. Sources inherit
`insecure_skip_verify` and the configured timeout.

From **Alert Details**, press **r** to find the generating rule. For vmalert,
the generator link must match a configured source's host, port, scheme, and
path prefix (hostnames are case-insensitive). A link ending in `/vmalert/alert`
can also match the configured base without the `/vmalert` suffix. PromTUI
uses the group and rule IDs to select the rule. If it cannot identify an exact
rule, Rules shows search results instead.

Rules refreshes every 30 seconds while the tab is open. Sources are fetched
independently: a failure leaves healthy sources visible, and a failed refresh
keeps the last successful data for that source marked **Stale**. Select the
source row to inspect its error.
