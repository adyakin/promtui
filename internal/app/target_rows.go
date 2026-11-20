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

type targetSort int

const (
	sortTargetHealth targetSort = iota
	sortTargetSource
	sortTargetJob
	sortTargetInstance
	sortTargetDuration
)

var targetSortNames = [...]string{"Health", "Source", "Job", "Instance", "Duration"}

type targetRow struct {
	api.Target
	id, source, job, instance string
	search                    string
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
			terms := []string{source, row.job, row.instance, item.ScrapeURL}
			for k, v := range labels {
				terms = append(terms, k, v, k+"="+v)
			}
			if dropped {
				for k, v := range item.Labels {
					terms = append(terms, k, v, k+"="+v)
				}
			}
			row.search = strings.ToLower(strings.Join(terms, "\x00"))
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

func compareTargetRows(a, b *targetRow, order targetSort) int {
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
