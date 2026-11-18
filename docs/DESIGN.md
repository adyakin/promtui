# PromTUI — Architecture & Design

**Status:** Initial design / pre-MVP
**Language:** Go
**Primary terminal:** Rex
**Backend:** Prometheus / VictoriaMetrics
**Visual reference:** https://github.com/rs/jplot

## 1. Project Overview

PromTUI is a keyboard-driven terminal application for exploring Prometheus-compatible metrics.

Its primary purpose is to provide an alternative to the Prometheus expression browser, especially in environments where accessing the monitoring web UI is inconvenient.

The application should feel similar to `k9s`, `lazygit`, or `pgcli`.

It is a **PromQL query explorer**, not a Grafana replacement.

### Design principles

- **Query-first:** PromQL editing and execution are the primary workflows.
- **Keyboard-first:** All essential operations work without a mouse.
- **High-quality graphs:** Prioritize readable terminal graphics over ASCII plots.
- **Rex-first:** Optimize for Rex before considering other terminals.
- **Minimal configuration:** A single binary with sensible defaults.
- **Responsive:** Network requests and rendering must not block the UI.
- **Small scope:** No dashboards, alerts, or monitoring configuration.
- **Backend compatibility:** Prometheus and VictoriaMetrics via their compatible HTTP APIs.

## 2. Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| TUI framework | `charmbracelet/bubbletea` |
| UI components | `charmbracelet/bubbles` |
| Styling | `charmbracelet/lipgloss` |
| PromQL parser | `prometheus/prometheus/promql/parser` |
| API client | Prometheus-compatible HTTP API |
| Graph rendering | Go image rendering libraries, TBD |
| Graphics transport | Sixel / DRCS, subject to validation |
| Sixel library candidate | `mattn/go-sixel` |
| Configuration | TOML |
| History | Local JSONL initially |
| Releases | GoReleaser |

### Library references

- https://github.com/charmbracelet/bubbletea
- https://github.com/charmbracelet/bubbles
- https://github.com/charmbracelet/lipgloss
- https://github.com/prometheus/prometheus
- https://github.com/mattn/go-sixel
- https://github.com/rs/jplot

Prefer established libraries over custom implementations.

The exact plotting and graphics libraries are intentionally undecided until the graphics feasibility spike is complete.

## 3. High-Level Architecture

```text
┌─────────────────────────────────────┐
│            Bubble Tea               │
│                                     │
│  PromQL Editor / Autocomplete       │
│  Query History                      │
│  Table / Graph Views                │
│  Connection / Time Controls         │
└──────────────────┬──────────────────┘
                   │
┌──────────────────▼──────────────────┐
│         Application Layer           │
│                                     │
│  Query Execution                    │
│  Async Requests / Cancellation      │
│  Result Transformation              │
│  Completion / Metadata Cache        │
└──────────────┬─────────────┬────────┘
               │             │
┌──────────────▼──────┐ ┌────▼─────────────┐
│ Prometheus API      │ │ Graph Renderer   │
│                     │ │                  │
│ Instant / Range     │ │ Time Series      │
│ Labels / Series     │ │ Axes / Legend    │
│ Metric Names        │ │ Raster Image     │
└─────────────────────┘ └──────┬───────────┘
                               │
                        ┌──────▼───────-----------┐
                        │ Kitty graphisc protocol │
                        │ Rex Terminal            │
                        └──────────────-----------┘
```

### Separation of concerns

Keep the following components independent:

- **API:** HTTP communication and response decoding.
- **Query:** Execution, cancellation and result normalization.
- **Editor:** Text editing, syntax awareness and completion.
- **History:** Persistent query storage and search.
- **Graph:** Plot layout and rasterization.
- **Terminal graphics:** Image encoding, placement and cleanup.
- **UI:** Bubble Tea application state and interactions.

Avoid unnecessary interfaces and abstraction layers.

The graph renderer must not depend directly on Bubble Tea or a particular terminal protocol.

## 4. User Interface

### Main screen

The interface contains three logical areas.

**Query editor**
- Multiline PromQL editing
- Syntax highlighting
- Context-aware autocomplete
- Query execution
- Searchable history

**Results**
- Table view
- Graph view
- Scrollable series and labels
- Error and warning display

**Status bar**
- Active connection
- Instant/range mode
- Selected time range
- Execution duration
- Request status

The interface should adapt to terminal resizing.

Avoid unnecessary panels, borders and decorative elements.

### Initial keyboard shortcuts

| Key | Action |
|---|---|
| `Ctrl+Enter` | Execute query |
| `Tab` | Accept completion |
| `Ctrl+Space` | Trigger completion |
| `Ctrl+R` | Search history |
| `Alt+Up/Down` | Navigate history |
| `Ctrl+C` | Cancel running query |
| `Esc` | Close popup / change focus |
| `?` | Show help |

These are proposed shortcuts, not hard requirements.

Verify keyboard event support in Rex and avoid conflicts with terminal behavior.

## 5. PromQL Editor

The editor is a core feature.

### MVP requirements

- Multiline editing
- Syntax highlighting
- Matching brackets and parentheses
- Autocomplete popup
- Persistent query history
- Copy/paste support
- Syntax error reporting

Use `bubbles/textarea` as an initial candidate, extending or replacing it only when necessary.

### Autocomplete

Support:

1. Metric names
2. PromQL functions and operators
3. Label names
4. Label values
5. Aggregation keywords and duration suggestions

Use the Prometheus PromQL parser where appropriate.

Fetch metric and label metadata from the backend.

Autocomplete requests must be asynchronous, debounced, cancellable and bounded.

Cache metadata to avoid unnecessary requests.

Do not automatically fetch all label values or enumerate all series.

### History

Store executed queries locally.

Support:
- Fuzzy search
- Deduplication
- Re-execution
- Timestamps
- Connection association

Use JSONL initially. Introduce SQLite only if needed.

## 6. Prometheus API

Use the Prometheus-compatible HTTP API.

### Required endpoints

```text
/api/v1/query
/api/v1/query_range
/api/v1/labels
/api/v1/label/<name>/values
/api/v1/series
/api/v1/metadata
```

Metric-name completion may use the `__name__` label values endpoint.

### Query modes

**Instant**
- Evaluate at a selected timestamp
- Display scalar, string and vector results

**Range**
- Relative time ranges: 15m, 1h, 6h, 24h, 7d
- Custom start/end timestamps
- Configurable or automatically calculated step
- Matrix results displayed as graphs or tables

### Execution requirements

- Asynchronous requests
- Context cancellation
- Configurable timeout
- Loading and error states
- Execution duration
- Configurable result limits
- Preserve last successful results when appropriate

Never block the UI event loop.

Handle Prometheus API warnings and partial responses explicitly.

## 7. Graph Rendering

**Graph quality is a primary project requirement.**

The desired result is a compact, readable time-series plot that makes effective use of terminal space.

We should use Kitty terminal graphics protocol

### Desired characteristics

- Multiple colored time series
- Readable time and value axes
- Grid lines
- Compact legends
- Automatic scaling
- Missing-sample handling
- Good use of available terminal space
- Fast redraws with minimal flicker

### Rendering pipeline

```text
Prometheus range query
        │
        ▼
Normalized time-series data
        │
        ▼
Graph layout / plotting
        │
        ▼
In-memory raster image
        │
        ▼
image encoder / transport
        │
        ▼
Rex terminal
```

Separate graph generation from terminal transport.

### Graphics protocol

Kitty graphics protocol is preferred as per spike-0 findings

Investigate:
- `mattn/go-sixel`
- Relevant rendering and terminal code in `rs/jplot`
- Compatibility with Bubble Tea's screen management

Do not assume Sixel support without testing.

If Sixel is unsuitable, investigate Kitty graphics protocol as the next candidate.

Do not build a generic multi-protocol graphics framework during the MVP.

### Graph interactions

MVP:
- Switch between table and graph
- Select a time range
- Show/hide individual series
- Resize with the terminal
- Display readable axes and legends

Deferred:
- Zoom and pan
- Cursor inspection
- Logarithmic scaling
- Image export

## 8. Table View

Requirements:

- Scrollable results
- Horizontal scrolling for long labels
- Expand/collapse metric labels
- Display timestamps and values
- Sort by value or metric name
- Copy selected values or labels
- Handle large result sets without freezing

Use `bubbles/table` as an initial candidate.

For range queries, provide a practical tabular representation without rendering every sample simultaneously.

## 9. Configuration

Example:

```toml
[general]
default_connection = "production"
history_limit = 1000

[ui]
theme = "auto"
graph_backend = "auto"

[query]
timeout = "30s"
default_range = "1h"
max_series = 500

[[connections]]
name = "production"
url = "http://victoriametrics:8428"

[[connections]]
name = "cluster"
url = "http://vmselect:8481/select/0/prometheus"
```

The connection URL is the API base URL.

Do not append duplicate API paths for VictoriaMetrics cluster endpoints.

Future support:
- Custom HTTP headers
- Basic authentication
- Bearer tokens
- TLS CA certificates
- Environment-variable credentials

Never store credentials in query history or logs.

## 10. Suggested Repository Structure

```text
promtui/
├── cmd/
│   └── promtui/
│       └── main.go
├── internal/
│   ├── app/
│   ├── api/
│   ├── editor/
│   ├── completion/
│   ├── history/
│   ├── query/
│   ├── graph/
│   │   ├── render/
│   │   └── sixel/
│   ├── ui/
│   │   ├── table/
│   │   └── graph/
│   └── config/
├── docs/
│   └── DESIGN.md
├── go.mod
└── README.md
```

This is a suggested structure, not a mandatory package hierarchy.

Avoid creating empty packages before implementation requires them.

## 11. Development Plan

### Spike 0 — Graphics feasibility

**Goal:** Determine whether high-quality Sixel graphs can be rendered reliably inside Rex and integrated with Bubble Tea.

Tasks:

1. Investigate Rex graphics capabilities.
2. Inspect jplot's graph and terminal rendering implementation.
3. Create a minimal standalone Sixel image prototype.
4. Render a sample multi-series time-series graph.
5. Test repeated redraws and image cleanup.
6. Test terminal resizing.
7. Integrate the prototype into a minimal Bubble Tea application.
8. Document findings and recommended implementation.

If Sixel fails, investigate Kitty graphics.

**Deliverable:** A runnable prototype and a short findings document.

Do not implement Prometheus API access, query editing or application features during this spike.

### Milestone 1 — Working query explorer

- Basic Bubble Tea interface
- Connection configuration
- Multiline query editor
- Instant and range queries
- Table results
- Error handling
- Persistent history

**Deliverable:** A useful interactive PromQL client.

### Milestone 2 — Editor experience

- Syntax highlighting
- Metric autocomplete
- Function autocomplete
- Label completion
- History search
- Syntax diagnostics

**Deliverable:** A polished PromQL editing experience.

### Milestone 3 — Graph integration

- Internal time-series model
- High-quality graph renderer
- Selected terminal graphics transport
- Table/graph switching
- Series visibility controls
- Resize handling

**Deliverable:** Integrated terminal graphs.

### Milestone 4 — Polish

- Improved navigation
- Series filtering
- Theme support
- Performance optimization
- Better connection management
- Documentation and release binaries

## 12. Testing

### Automated tests

- API response parsing
- Query result transformations
- Completion context
- History persistence
- Time-range calculations
- Graph scaling and missing samples

### Integration tests

Use local Prometheus or VictoriaMetrics.

Test:
- Instant/range queries
- Empty results
- Invalid PromQL
- HTTP failures
- Timeouts
- Large result sets

### Terminal testing

**MVP target: Rex only.**

Test local execution, rendering, keyboard behavior and resizing.

SSH, iTerm2 and other terminal compatibility testing are deferred.

## 13. Non-Goals

Explicitly out of scope:

- Grafana dashboard replacement
- Dashboard editing
- Alert management
- Scrape configuration
- Recording rule management
- Metric ingestion or storage
- Multi-user functionality
- Web frontend
- Plugin framework
- AI-assisted query generation

Do not introduce these without an explicit design decision.

## 14. Guidance for Coding Agents

- Implement incrementally.
- Keep the application runnable.
- Prioritize a functional MVP over architectural perfection.
- Use maintained Go libraries.
- Avoid reinventing PromQL parsing.
- Avoid unnecessary abstractions.
- Keep networking and rendering asynchronous.
- Write tests for nontrivial logic.
- Validate terminal assumptions experimentally.
- Prefer small, reviewable changes.
- Document significant architectural decisions.
- Do not expand the project scope without approval.

**Success criterion:** Executing, exploring and visualizing arbitrary PromQL queries in Rex should be easier and faster than opening the Prometheus web UI.

