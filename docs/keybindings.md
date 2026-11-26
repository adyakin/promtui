# Keyboard shortcuts

Shortcuts depend on the active pane, the focused input, and any open dialog.
Uppercase keys such as `P`, `D`, and `Y` require Shift. On macOS, configure
your terminal to send Alt for Option shortcuts. PromTUI does not bind
function keys. Some actions require data or a selected item before they work.

[Navigation](#navigation) · [PromQL](#0-promql) · [Alerts](#1-alerts) ·
[Silences](#2-silences) · [Targets](#3-targets) · [Routing](#4-routing) ·
[Rules](#5-rules) · [Parameters](#saved-item-parameters) · [Text editing](#text-editing)

## Navigation

| Where | Keys | Action |
| --- | --- | --- |
| Main views, outside text input | `0` / `1` / `2` / `3` / `4` / `5` | Open PromQL / Alerts / Silences / Targets / Routing / Rules. |
| Where tab navigation is available, even while typing | `Alt+0`–`Alt+5` | Switch tabs without typing a digit. |
| Outside text input, with connections configured | `c` | Open the connection picker. |
| Where the connection picker is available, even while typing | `Alt+C` | Open the connection picker. |
| Connection picker | `↑` / `↓`; `Enter`; `Esc` | Select; switch; dismiss without switching. |
| Main views | `Ctrl+C` | Cancel a pending request when possible; otherwise quit. |
| Main views and most dialogs | `Ctrl+Q` | Quit. Parameter prompts and in-progress silence writes capture input and don't accept this shortcut. |
| Data-pane lists and PromQL results | `q` | Quit. |

Overlays can capture tab, connection, and cancel shortcuts. Dismiss the
overlay with `Esc` before switching if the shortcut isn't available.

## 0 PromQL

| Mode | Keys | Action |
| --- | --- | --- |
| Editor, completion, Table, Graph | `Ctrl+Enter` / `Ctrl+G` | Run in the current query mode. |
| Editor, Table, Graph | `Ctrl+T` | Switch between instant and range modes; runs unless a query error is being shown. |
| Editor | `Tab` / `Shift+Tab`; `Esc` | Focus results. In completion, `Tab` accepts the suggestion instead, `Shift+Tab` does nothing, and `Esc` dismisses completion. |
| Editor, completion | `Enter`; `Alt+↑` / `Alt+↓` | Insert a newline; recall older / newer history. |
| Editor, completion | `Ctrl+Space` / `Ctrl+@`; `Ctrl+R` | Request completion; open history search. |
| Editor, completion | `Ctrl+S` | Save the current expression. |
| Editor, completion, Table, Graph | `Ctrl+O` | Open Saved Queries. |
| Completion | `↑` / `↓`; `Tab` | Select a suggestion; accept it. |
| History search | Type / `Backspace`; `↑` / `↓`; `Enter`; `Esc` | Filter history; select; restore to editor without running; close. |
| Table, Graph | `i` / `Tab` / `Shift+Tab` | Focus the editor. |
| Table, Graph | `g`; `t`; `r` / `Ctrl+W` | Switch to range graph or instant table and run (unless a query error is being shown); open the range picker. |
| Table, Graph | `s`; `/`; `↑` / `↓` | Open the graph series selector; edit the result filter; select a row or series. |
| Table, Graph | `l`; `y`; `Y` | Inspect full labels; copy the selected value; copy the selected series' labels. |
| Table, Graph | `e`; `?` | Open error details after a failed query; open help. |
| Editor, completion, Table, Graph, Help, Status | `Alt+S` | Open or close request status. |
| Table | `PgUp` / `PgDn`; `Home` / `End`; `←` / `→` | Page rows; jump to first or last row; pan columns. |
| Table | `Enter`; `n` / `v`; `[` / `]` | Expand or collapse the table; sort by name or value; move to an earlier or later sample of the selected series. |
| Graph | `Space` | Show or hide the selected series. |
| Range picker | `↑` / `↓`; `Enter`; `Esc` | Select a range; apply and run; close. |
| Series selector | `↑` / `↓`; `Space`; `Enter`; `a`; `/`; `Esc` | Select; toggle visibility; isolate the selected series or restore visibility; show all; filter; close. |
| Result or series filter input | Type / `Backspace`; `Enter`; `Esc` | Edit the filter; apply; cancel the edit (`Esc` also closes the series selector when editing its filter). |
| Help, Status, Labels, Query Error | `↑` / `↓` / `PgUp` / `PgDn`; `Esc` | Scroll; close. |
| Help and Status | `?` | Toggle Help. |
| Labels; Query Error | `y` | Copy the displayed labels or error. |

### Saved Queries

| Mode | Keys | Action |
| --- | --- | --- |
| Browser | `↑` / `↓`; `/`; `Enter`; `Esc` | Select; search; load without running (or open the parameter prompt); close. |
| Browser, selected query | `e`; `u`; `t`; `d` | Rename; update from the current expression (or edit the existing template); edit template YAML; delete after confirmation. |
| Browser search | `↑` / `↓`; `Enter`; `Esc` | Select; accept the search; cancel the search edit. |
| Name or confirmation | `Enter`; `Esc` | Confirm; cancel. |
| Template editor | `Ctrl+Enter` / `Ctrl+G`; `Esc` | Save the YAML template; cancel. |

## 1 Alerts

| Mode | Keys | Action |
| --- | --- | --- |
| List | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` | Navigate alerts, or scroll a pane error. |
| List | `/`; `s`; `r`; `Enter`; `?` | Search; cycle sorting; refresh; open Alert Details; open Help. |
| List | `m`; `x`; `Ctrl+S`; `Ctrl+O`; `D` | Edit label matchers; clear matchers only; save matchers as a filter; browse filters and views; restore the connection's default view. |
| Search | `↑` / `↓`; `Enter`; `Esc` | Select an available alert-name suggestion; apply search, using the highlighted suggestion if any; cancel. |
| List, Search, Details, Related Alerts, Help, Status | `Alt+S` | Open or close Alerts status. |
| Details | `p` / `P` / `r`; `s` | Run a historical graph / load the original expression without running / find the generating rule; draft a silence from the alert labels. |
| Details | `i`; `y`; `u` | Open related alerts when available; copy labels; copy the generator URL. |
| Related Alerts | `↑` / `↓` / `PgUp` / `PgDn`; `Enter`; `Esc` | Select; follow a related alert; return to details. |
| Details, Help, Status | `↑` / `↓` / `PgUp` / `PgDn`; `Esc` | Scroll; close or go back through followed alerts. |
| Details, Related Alerts, Help, Status | `?` | Toggle Help. |

### Saved filters, views, and matcher editor

| Mode | Keys | Action |
| --- | --- | --- |
| Picker | `↑` / `↓` / `PgUp` / `PgDn`; `/`; `Enter`; `Esc` | Select; search; open a filter or view; close. |
| Picker, selected filter only | `e`; `u`; `t`; `d` | Rename; replace with active matchers (or edit the existing template); edit filter YAML; delete after confirmation. |
| Picker search | `↑` / `↓` / `PgUp` / `PgDn`; `Enter`; `Esc` | Select; accept the search; cancel the search edit. |
| Matcher editor | `↑` / `↓`; `Tab` / `Shift+Tab`; `Ctrl+N` / `Ctrl+D` | Move between rows; move between fields; add or remove a row. |
| Matcher editor, operator field | `←`; `→` / `Space` / `Enter` | Cycle the operator backward or forward. |
| Matcher editor | `Ctrl+Enter` / `Ctrl+G`; `Esc` | Apply matchers locally; cancel. |
| Name or confirmation | `Enter`; `Esc` | Confirm; cancel. |
| Template editor | `Ctrl+Enter` / `Ctrl+G`; `Esc` | Save filter YAML; cancel. |

## 2 Silences

The same draft shortcuts apply when opening from Alert Details.

| Mode | Keys | Action |
| --- | --- | --- |
| List | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` | Select a silence. |
| List | `/`; `s`; `e`; `r`; `n`; `Enter`; `?` | Search; cycle sorting; include or hide expired silences; refresh; create a draft; open details; open Help. |
| Search | `Enter`; `Esc` | Apply or cancel search. |
| Details | `a`; `e`; `x`; `Esc` | Inspect matching alerts; edit; request expiration; close. |
| Draft | `Tab` / `Shift+Tab`; `Enter` | Focus next or previous field; activate the focused button (schedule, operator, add/remove matcher, or preview). |
| Draft | `Ctrl+P` (or `p` on a button); `Ctrl+Enter` / `Ctrl+G`; `Esc` | Preview matching alerts; submit; discard the draft. |
| Preview or matching-alert list | `Esc` | Return to the draft or details. |
| Broad matcher or expiration confirmation | `Enter`; `Esc` | Confirm; cancel. |
| Details, preview, matching alerts, confirmation, Help | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` | Scroll. |
| Help | `Esc` / `?` | Close. |

## 3 Targets

| Mode | Keys | Action |
| --- | --- | --- |
| List | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` | Navigate targets, or scroll a pane error. |
| List | `/`; `f`; `h`; `d`; `s` | Search; choose a scrape source; cycle health; toggle dropped targets; cycle sorting. |
| List | `Enter`; `r`; `?` | Open details; refresh; open Help. |
| Search | `Enter`; `Esc` | Apply or cancel search. |
| Source picker | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End`; `Enter`; `Esc` | Select a source; apply; close unchanged. |
| List, Search, Details, Help, Status | `Alt+S` | Open or close target request status. |
| Details | `y`; `u` | Copy labels; copy the scrape URL (or the global URL if the scrape URL isn't an HTTP(S) link). |
| Details, Help, Status | `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End`; `Esc`; `?` | Scroll; close; toggle Help. |

## 4 Routing

| Mode | Keys | Action |
| --- | --- | --- |
| Routes and Receivers | `v`; `/`; `r`; `?` | Switch views; search; refresh; open Help. |
| Routes and Receivers | `Tab` / `Shift+Tab`; `↑` / `↓` (or `k` / `j`) / `PgUp` / `PgDn` / `Home` / `End` | Switch focus between list and details; navigate or scroll the focused pane. |
| Routes tree, not searching | `→` / `l`; `←` / `h` | Expand; collapse or move to the parent. |
| Routes / Receivers | `Enter`; `y` | View the route's receiver / find routes referencing the receiver; copy the selected receiver YAML. |
| Receivers only | `s` | Toggle alphabetical sorting. |
| Search | `Enter`; `Esc` | Apply or cancel search. |
| Receiver YAML dialog | `y`; `Esc` | Copy receiver YAML; close. |
| References dialog | `Enter`; `Esc` | Jump to the selected route; close. |
| Receiver YAML, References, Help | `↑` / `↓` (or `k` / `j`) / `PgUp` / `PgDn` / `Home` / `End` | Scroll text or select a reference. |
| Help | `?` / `Esc` | Close. |

## 5 Rules

| Mode | Keys | Action |
| --- | --- | --- |
| Tree and details | `/`; `Esc`; `r`; `?` | Search; clear the applied search; refresh; open Help. |
| Tree and details | `f`; `s`; `Tab` / `Shift+Tab` | Cycle rule type; cycle alert state; switch focus between tree and details. |
| Tree and details | `↑` / `↓` (or `k` / `j`) / `PgUp` / `PgDn` / `Home` / `End` | Navigate or scroll the focused pane. |
| Tree | `→` / `l`; `←` / `h`; `Enter` | Expand or move to a child; collapse or move to the parent; toggle a branch or focus details for a leaf. |
| Tree and details | `p`; `e` | Load the selected rule's expression into PromQL without running; open the selected error. |
| Details with an error | `Enter` | Open the error overlay. |
| Search | `Enter`; `Esc` | Apply or cancel search. |
| Error overlay, Help | `↑` / `↓` (or `k` / `j`) / `PgUp` / `PgDn` / `Home` / `End`; `Esc` | Scroll; close. |
| Help | `?` | Close. |

## Saved-item parameters

| Keys | Action |
| --- | --- |
| `Tab` / `↓`; `Shift+Tab` / `↑` | Move to the next or previous parameter. |
| `Ctrl+Space` / `Ctrl+@`; `Ctrl+V` | Explicitly use an empty value; paste. |
| `Ctrl+Enter` / `Ctrl+G`; `Esc` | Apply the values; return without applying. |

## Text editing

These editing keys apply to focused fields unless a mode above uses the same
key. Search buffers in the PromQL history and series selector accept typed text,
paste, and `Backspace`; they are not full text editors.

| Focus | Keys | Action |
| --- | --- | --- |
| Text inputs and multiline editors | `←` / `Ctrl+B`; `→` / `Ctrl+F` | Move left or right. |
| Text inputs and multiline editors | `Alt+←` / `Ctrl+←` / `Alt+B`; `Alt+→` / `Ctrl+→` / `Alt+F` | Move backward or forward by word. |
| Text inputs and multiline editors | `Home` / `Ctrl+A`; `End` / `Ctrl+E` | Move to start or end of line. |
| Text inputs and multiline editors | `Backspace` / `Shift+Backspace` / `Ctrl+H`; `Delete` / `Ctrl+D` | Delete the preceding or following character. |
| Text inputs and multiline editors | `Alt+Backspace` / `Ctrl+Backspace` / `Ctrl+W`; `Alt+Delete` / `Ctrl+Delete` / `Alt+D` | Delete the preceding or following word. |
| Text inputs and multiline editors | `Ctrl+U`; `Ctrl+K`; `Ctrl+V` | Delete before cursor; delete after cursor; paste. |
| Multiline editors | `↑` / `Ctrl+P`; `↓` / `Ctrl+N`; `PgUp` / `PgDn` | Move up or down by line or page. |
| Multiline editors | `Enter` / `Ctrl+M`; `Ctrl+Home` / `Alt+<`; `Ctrl+End` / `Alt+>` | Insert a newline; go to beginning or end of the input. |
| Multiline editors | `Shift+←` / `Shift+→`; `Shift+↑` / `Shift+↓` | Extend selection by character or line. |
| Multiline editors | `Ctrl+Shift+←` / `Ctrl+Shift+→`, `Alt+Shift+←` / `Alt+Shift+→`, or `Alt+Shift+B` / `Alt+Shift+F` | Extend selection by word. |
| Multiline editors | `Ctrl+Shift+C`; `Alt+L` / `Alt+U` | Copy selection; lowercase or uppercase the word ahead. |
| Multiline editors, when not used by a pane shortcut | `Alt+C`; `Ctrl+T` | Capitalize the word ahead; transpose characters. |
