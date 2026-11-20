# PromTUI — UI/UX Design Specification

**Status:** Proposed design
**Priority:** Rex terminal
**Scope:** User interface, interaction model, keyboard navigation and visual organization
**Related documents:** `docs/DESIGN.md`, `docs/SPIKE-0.md`

## 1. Overview

PromTUI is a keyboard-driven PromQL query explorer for Prometheus and VictoriaMetrics.

The interface should combine the simplicity of the Prometheus expression browser with the responsiveness and discoverability of modern terminal applications such as `k9s` and `lazygit`.

The primary goal is to make querying and exploring metrics intuitive without requiring users to memorize numerous keyboard shortcuts.

**The interface must be discoverable, predictable and consistent.**

### Core UX principles

1. **Discoverability over memorization.** Users should see available actions without opening help.
2. **Context-sensitive controls.** Shortcuts depend on the active component.
3. **Predictable focus.** Navigation between components must be explicit and consistent.
4. **Minimal visual noise.** Maximize space for queries and results.
5. **Keyboard-first.** Every essential action must be available through the keyboard.
6. **Visible selection.** Any action targeting an item must clearly indicate which item is selected.
7. **Non-destructive navigation.** Switching views or opening overlays must not discard queries or results.
8. **Responsive feedback.** User actions should produce immediate visual feedback.

The application should feel natural to users familiar with terminal tools, without requiring knowledge of Vim.

---

## 2. Interaction Model

The application has three primary interaction contexts.

### 2.1 Query Editor

The user is editing a PromQL expression.

Behavior:
- Ordinary keys insert text.
- Multiline editing is supported.
- PromQL completion is available.
- Query history can be opened.
- Executing a query does not automatically change focus.
- Results update asynchronously.
- The current query remains editable after execution.

Single-character application shortcuts must not interfere with typing.

For example, `q`, `g`, `s`, `r` and `t` must behave as ordinary text inside the editor.

### 2.2 Results

The user is inspecting a table or graph.

Behavior:
- Single-character navigation shortcuts are enabled.
- Results can be explored without changing the query.
- Table/graph switching preserves the current result.
- Series visibility can be managed.
- Time ranges can be changed.
- The query editor can be focused explicitly.

### 2.3 Overlays

Temporary interactive views, including:

- Query history
- Autocomplete suggestions
- Series selector
- Time-range selector
- Help
- Status and warnings

Overlays should have a clear title, visible selection or input focus, and context-sensitive footer.

Closing an overlay returns to the component that opened it.

Nested overlays should be avoided unless genuinely necessary.

---

## 3. Focus Management

Focus management is one of the most important aspects of the interface.

### 3.1 Focus rules

- Application starts with the Query Editor focused.
- `Tab` and `Shift+Tab` switch between Query Editor and Results.
- `i` in Results is an alias for returning to the Query Editor.
- Running a query focuses Results immediately; the response does not change focus.
- Opening an overlay temporarily captures focus.
- Closing an overlay restores the previous focus.
- Focus must be visually obvious.

### 3.2 Escape behavior

**Esc means Back, Cancel or Close. It must never cycle between components.**

Expected behavior:

| Context | Esc action |
|---|---|
| Autocomplete | Close suggestions |
| History | Close without restoring a query |
| Series selector | Close and return to Results |
| Time-range selector | Close without applying changes |
| Help | Close |
| Status overlay | Close |
| Query Editor | No action |
| Results | No action |

Esc dismisses autocomplete without leaving the Query Editor.

### 3.3 Focus indicators

Use a clear visual indicator for the focused component.

Possible techniques:
- Highlighted section title
- Colored border or accent
- Visible cursor in editor
- Selected row in tables or lists

Do not rely on color alone to indicate focus.

---

## 4. Keyboard Shortcuts

### 4.1 Global shortcuts

Global shortcuts must work regardless of the focused component, except where a terminal or active overlay prevents them.

| Key | Action |
|---|---|
| `Ctrl+G` | Execute query |
| `Ctrl+Enter` | Execute query, if supported |
| `?` | Context-sensitive help outside text inputs |
| `Alt+S` | Request status |
| `Ctrl+Q` | Quit application |
| `Ctrl+C` | Cancel active query, or quit if idle |
| `Esc` | Context-dependent Back/Close |

While a query is running, `Ctrl+C` cancels it without quitting. Press `Ctrl+C` again to quit.

Use `Ctrl+G` on terminals that cannot distinguish `Ctrl+Enter` from `Enter`. Do not bind function keys.

### 4.2 Query Editor shortcuts

| Key | Action |
|---|---|
| `Ctrl+G` | Execute query |
| `Ctrl+Enter` | Execute query, if supported |
| `Enter` | Insert newline |
| `Tab` / `Shift+Tab` | Focus Results |
| `Ctrl+Space` | Open autocomplete |
| `Ctrl+R` | Search query history |
| `Alt+Up` | Previous query |
| `Alt+Down` | Next query |

When autocomplete is open, Tab accepts the selected suggestion rather than changing focus.

History navigation must preserve an unfinished query so it can be restored.

### 4.3 Results shortcuts

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` / `i` | Focus Query Editor |
| `g` | Run a range query in Graph |
| `s` | Open Series Selector |
| `r` | Choose a time range; Enter applies it and reruns |
| `t` | Run an instant query in Table |
| `l` | Show selected series labels |
| `y` | Copy selected value |
| `?` | Help |
| `q` | Quit |
| `Ctrl+Enter` / `Ctrl+G` | Re-execute query |

Single-character shortcuts are active only in Results or appropriate overlays.

### 4.4 Table navigation

| Key | Action |
|---|---|
| `↑ / ↓` | Select previous/next row |
| `PgUp / PgDn` | Scroll one page |
| `Home / End` | First/last row |
| `← / →` | Horizontal scroll |
| `Enter` | Expand/collapse selected series |
| `n` | Sort by name |
| `v` | Sort by value |
| `y` | Copy selected value |
| `Y` | Copy selected labels |

The selected row must remain visible when scrolling.

### 4.5 Graph navigation

| Key | Action |
|---|---|
| `↑ / ↓` | Select previous/next series |
| `s` | Open Series Selector |
| `Space` | Toggle selected series visibility |
| `g` | Switch to Table |
| `r` | Change time range |
| `l` | Inspect selected series labels |

The selected series must be visibly identified before Space can toggle it.

Do not silently toggle the first series when no selection is visible.

---

## 5. Context-Sensitive Footer

**The context-sensitive footer is a core application feature and must be preserved.**

The footer displays shortcuts relevant to the current focus or overlay.

It is the primary mechanism for discovering available actions.

### 5.1 General behavior

- Always visible at the bottom of the terminal.
- Changes automatically when focus changes.
- Displays the most relevant actions first.
- Uses concise labels.
- Does not attempt to display every shortcut.
- Never advertises unavailable actions.
- Updates when an overlay opens or closes.

### 5.2 Example: Query Editor

```text
^Enter Run   Tab Results   ^R History   ^Space Complete
```

### 5.3 Example: Graph

```text
Tab Query   g Graph   t Table   s Series   r Range   q Quit   ? Help
```

### 5.4 Example: Table

```text
Tab Query   g Graph   t Table   r Range   q Quit   ↑↓ Select   Enter Expand
```

### 5.5 Example: Series Selector

```text
↑↓ Select   Space Toggle   Enter Isolate   a All   / Filter   Esc Back
```

### 5.6 Example: History

```text
Type Search   ↑↓ Select   Enter Restore   Esc Cancel
```

### 5.7 Footer design

- Prefer one line.
- Use two lines only when terminal width is insufficient and wrapping is genuinely useful.
- Avoid clutter.
- Highlight key combinations.
- Use consistent naming.
- Prioritize actions by relevance rather than alphabetical order.

### 5.8 Implementation requirement

Keyboard actions should be defined centrally.

The same action definitions should drive:

1. Key handling
2. Footer labels
3. Help screens

Avoid maintaining separate, inconsistent lists of shortcuts.

---

## 6. Main Screen Layout

The main screen should prioritize the PromQL editor and query results.

### 6.1 Layout structure

```text
┌─────────────────────────────────────────────────────┐
│ PromTUI │ production           Range · Last 6h · UTC │
├─────────────────────────────────────────────────────┤
│ QUERY                                 ^Enter Run     │
│                                                     │
│ 100 * (1 - avg by (instance) (                      │
│   rate(node_cpu_seconds_total{mode="idle"}[5m])     │
│ ))                                                  │
├─────────────────────────────────────────────────────┤
│ GRAPH                                 6 series      │
│                                                     │
│                 Time-series graph                   │
│                                                     │
│                                                     │
│                                                     │
│                                                     │
│                                                     │
│                                                     │
│                                                     │
│                                                     │
├─────────────────────────────────────────────────────┤
│ ● server01  ● server02  ● server03   +3 more         │
├─────────────────────────────────────────────────────┤
│ Done · 85ms · 6 series                               │
│ Tab Query   g Graph   t Table   r Range   q Quit    │
└─────────────────────────────────────────────────────┘
```

This is a conceptual layout, not a strict character-by-character rendering specification.

### 6.2 Header

Display:
- Application name
- Active connection
- Instant/Range mode
- Selected time range
- Timezone

Avoid repeating this information in multiple locations.

### 6.3 Query Editor

Requirements:
- Multiline editing
- Syntax highlighting
- Line numbers where useful
- Visible focus state
- Automatic height
