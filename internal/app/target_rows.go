package app

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"promtui/internal/api"
	"promtui/internal/config"
)

type targetHealth int

const (
	targetDown targetHealth = iota
	targetUnknown
	targetUp
	targetDropped
	targetAllActive
)

var targetHealthNames = [...]string{"DOWN", "UNKNOWN", "UP", "DROPPED", "Active"}
var targetHealthOrder = [...]targetHealth{targetAllActive, targetDown, targetUp, targetUnknown, targetDropped}

const (
	sortTargetHealth   = "health"
	sortTargetSource   = "source"
	sortTargetJob      = "job"
	sortTargetInstance = "instance"
	sortTargetDuration = "duration"
)

var defaultTargetSorts = []viewSort{
	{sortTargetHealth, "Health"}, {sortTargetSource, "Source"}, {sortTargetJob, "Job"},
	{sortTargetInstance, "Instance"}, {sortTargetDuration, "Duration"},
}

var defaultTargetFields = [...]string{"health", "instance", "job", "source", "last_scrape", "duration"}

type targetRow struct {
	api.Target
	id, source, job, instance string
	search                    []string
	health                    targetHealth
	sourceIndex               int
	lastScrape                time.Time
}

func makeTargetRows(source string, targets api.Targets) []targetRow {
	rows := make([]targetRow, 0, len(targets.Active)+len(targets.Dropped))
	first := make(map[string]int)
	duplicates := make(map[string][]int)
	for _, dropped := range []bool{false, true} {
		items := targets.Active
		if dropped {
			items = targets.Dropped
		}
		for _, item := range items {
			row := targetRow{Target: item, source: source, health: targetUnknown}
			labels := item.Labels
			if dropped {
				row.health, labels = targetDropped, item.DiscoveredLabels
			} else {
				switch strings.ToLower(item.Health) {
				case "up":
					row.health = targetUp
				case "down":
					row.health = targetDown
				}
				row.lastScrape, _ = time.Parse(time.RFC3339Nano, item.LastScrape)
			}
			row.job, row.instance = labels["job"], labels["instance"]
			if dropped && row.instance == "" {
				row.instance = labels["__address__"]
			}
			// JSON sorts label keys and safely delimits each identity component.
			key, _ := json.Marshal([]any{source, dropped, item.ScrapePool, item.ScrapeURL, labels})
			base := string(key) + ":"
			row.id = base + "0"
			if j, ok := first[base]; ok {
				if duplicates[base] == nil {
					duplicates[base] = []int{j}
				}
				duplicates[base] = append(duplicates[base], len(rows))
			} else {
				first[base] = len(rows)
			}
			row.search = []string{source, targetHealthNames[row.health], item.Health, item.ScrapePool,
				item.ScrapeURL, item.GlobalURL, item.LastError, item.LastScrape, item.ScrapeInterval, item.ScrapeTimeout}
			if item.LastScrapeDuration != nil {
				row.search = append(row.search, targetDuration(item.LastScrapeDuration),
					strconv.FormatFloat(*item.LastScrapeDuration, 'f', -1, 64))
			}
			rows = append(rows, row)
		}
	}
	for base, indices := range duplicates {
		keys := make(map[int]string, len(indices))
		for _, i := range indices {
			key, _ := json.Marshal([]any{rows[i].DiscoveredLabels, rows[i].Labels, rows[i].GlobalURL})
			keys[i] = string(key)
		}
		slices.SortStableFunc(indices, func(i, j int) int { return cmp.Compare(keys[i], keys[j]) })
		for n, i := range indices {
			rows[i].id = base + strconv.Itoa(n)
		}
	}
	return rows
}

func (r *targetRow) matches(f fuzzyFilter) bool {
	return f.matches(r.search...) || matchLabels(f, r.Labels) || matchLabels(f, r.DiscoveredLabels)
}

func (r *targetRow) field(field string) string {
	if label, ok := config.LabelName(field); ok {
		return r.Labels[label]
	}
	value, _ := r.builtInField(field)
	return value
}

func (r *targetRow) searchField(field string) string {
	if label, ok := config.LabelName(field); ok {
		return r.Labels[label]
	}
	if strings.HasPrefix(field, "annotations.") {
		return ""
	}
	if value, ok := r.builtInField(field); ok {
		return value
	}
	if value, ok := r.Labels[field]; ok {
		return value
	}
	if r.health == targetDropped {
		return r.DiscoveredLabels[field]
	}
	return ""
}

func (r *targetRow) builtInField(field string) (string, bool) {
	switch field {
	case "health":
		return targetHealthNames[r.health], true
	case "source":
		return r.source, true
	case "job":
		return r.job, true
	case "instance":
		return r.instance, true
	case "last_scrape", "lastScrape":
		return r.LastScrape, true
	case "duration", "lastScrapeDuration":
		if r.LastScrapeDuration != nil {
			return targetDuration(r.LastScrapeDuration), true
		}
		return "", true
	case "scrape_url", "scrapeUrl":
		return r.ScrapeURL, true
	case "scrape_pool", "scrapePool":
		return r.ScrapePool, true
	case "global_url", "globalUrl":
		return r.GlobalURL, true
	case "last_error", "lastError":
		return r.LastError, true
	case "scrape_interval", "scrapeInterval":
		return r.ScrapeInterval, true
	case "scrape_timeout", "scrapeTimeout":
		return r.ScrapeTimeout, true
	}
	return "", false
}

func (r *targetRow) displayField(field string, now time.Time) string {
	if field == "last_scrape" {
		return targetScrapeAge(r.lastScrape, now)
	}
	return r.field(field)
}

func compareTargetRows(a, b *targetRow, order string) int {
	var primary int
	switch order {
	case sortTargetSource:
		primary = cmp.Compare(a.source, b.source)
	case sortTargetJob:
		primary = cmp.Compare(a.job, b.job)
	case sortTargetInstance:
		primary = cmp.Compare(a.instance, b.instance)
	case sortTargetDuration:
		x, y := a.LastScrapeDuration, b.LastScrapeDuration
		switch {
		case x != nil && y != nil:
			primary = cmp.Compare(*y, *x)
		case x != nil:
			primary = -1
		case y != nil:
			primary = 1
		}
	case "last_scrape":
		primary = cmp.Or(missingLast(a.lastScrape.IsZero(), b.lastScrape.IsZero()), b.lastScrape.Compare(a.lastScrape))
	case "scrape_interval", "scrape_timeout":
		x, y := a.field(order), b.field(order)
		dx, ex := time.ParseDuration(x)
		dy, ey := time.ParseDuration(y)
		if ex == nil && ey == nil {
			primary = cmp.Compare(dx, dy)
		} else {
			// Group parseable values to keep the comparison transitive.
			primary = cmp.Or(missingLast(ex != nil, ey != nil), compareFieldText(x, y))
		}
	case sortTargetHealth:
		// Health is the first tiebreaker for every sort mode.
	default:
		primary = compareFieldText(a.field(order), b.field(order))
	}
	return cmp.Or(primary, cmp.Compare(a.health, b.health), cmp.Compare(a.source, b.source),
		cmp.Compare(a.job, b.job), cmp.Compare(a.instance, b.instance), cmp.Compare(a.id, b.id))
}

func targetLabels(labels map[string]string) []string {
	lines := make([]string, 0, len(labels))
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		lines = append(lines, k+"="+strconv.Quote(labels[k]))
	}
	return lines
}
