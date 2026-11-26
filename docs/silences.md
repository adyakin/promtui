# Managing Alertmanager silences

Open Silences with **2**. It uses the selected connection’s `alertmanager_url`
and TLS settings.
It works even if the connection’s metrics endpoint is unavailable.

## Browse and inspect

- **2** opens Silences. The list starts with active and pending records;
  **e** includes expired ones. The counts always include all fetched records.
- **/** searches matcher names and values, creator, comment, and status. Use
  **Enter** to apply the search or **Esc** to cancel. **s** cycles through
  status, end time, and creator sorts.
- **Enter** opens Details; **a** shows currently matching alerts. Use the
  arrow or page keys to scroll, and **Esc** to go back.
- **r** refreshes immediately. Polling is off by default; opt in with
  [`auto_refresh.silences`](configuration.md#pane-auto-refresh). Silence
  start and expiry states update locally even when polling is off.

## Create a silence

1. In **2 Silences**, press **n**. The default start is **Now** and the
   default duration is **2h**.
2. Use **Tab** and **Shift+Tab** to navigate and **Enter** to activate a
   focused button. Set the creator and comment, then add a matcher such as
   `job="billing"`. Choose an operator by focusing it and pressing **Enter**.
3. Press **Ctrl+P** (or **p** while on a button) to preview. **Esc** returns
   to the same draft. Changing a matcher invalidates the preview count.
4. Press **Ctrl+Enter** (or **Ctrl+G**) to create. PromTUI asks for
   confirmation if matchers are broad, for example if they can match an
   empty or missing label. A failed request leaves the draft open.

To set a default creator, add this optional top-level setting to
`config.yaml`. It stays editable in the draft:

```yaml
silence_creator: on-call
```

## Silence an existing alert

In **1 Alerts**, select an alert, press **Enter**, then **s**. This opens
the same editor with exact equality matchers for every actual alert label.
Remove labels that are too specific, review the preview, then create the
silence. **Esc** from the draft returns to the original Alert Details.
Successful creation returns to Alerts and refreshes its statuses. **s** in the
Alerts list still sorts.

## Matchers and preview

| Operator | Meaning |
| --- | --- |
| `=` | Equals |
| `!=` | Does not equal |
| `=~` | Matches the full value as a regular expression |
| `!~` | Does not match the full value as a regular expression |

All matchers must match. A missing label is evaluated as an empty string.
Values are literal, including leading and trailing spaces. Values containing
tabs or newlines are shown as double-quoted strings with escapes so they can
be edited on one line.
The preview checks currently known Alertmanager alerts, including silenced and
inhibited alerts. A count of zero does not mean the silence cannot match
future alerts.

## Schedule, edit, or expire

Activate the **Start** button to switch from **Now** to a future local date
and the **End** button to switch from a **Duration** to an absolute local date.
Enter dates as `YYYY-MM-DD HH:MM`. The editor shows the local timezone or UTC
offset; ambiguous or nonexistent times during a clock change are rejected.
The **Now** time is resolved at submission. Durations use Go syntax, with
`d` supported as a fixed 24 hours (for example, `7d`).

In Silence Details, **e** edits an active or pending silence. Alertmanager
can expire the old record and return a replacement ID if a change affects
its history; PromTUI follows that ID. On an expired record, **e** opens a new
draft with the old matchers, creator, and comment. **x** opens the expiration
confirmation for an active or pending silence; **Enter** confirms. Expiration
keeps the record in history.

Silence dialogs block tab and connection switching until you close them.
PromTUI checks for changes to a record before editing or expiring it, but
Alertmanager v2 does not offer atomic conditional writes. If a write times
out, refresh before trying again: Alertmanager may already have applied it.
