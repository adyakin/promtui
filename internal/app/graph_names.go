package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/prometheus/prometheus/promql/parser"
	"promtui/internal/query"
	"promtui/internal/ui"
)

var promQLParser = parser.NewParser(parser.Options{})

func graphSeriesNames(series []query.Series) []string {
	names := make([]string, len(series))
	labels := make([]map[string]string, len(series))
	groups := make(map[string][]int)
	for i, s := range series {
		labels[i] = make(map[string]string, len(s.Metric))
		for k, v := range s.Metric {
			labels[i][string(k)] = strings.TrimSpace(ui.Plain(string(v)))
		}
		if len(labels[i]) == 0 && s.Labels != "" && s.Labels != "{}" {
			if metric, err := promQLParser.ParseMetric(s.Labels); err == nil {
				for k, v := range metric.Map() {
					labels[i][k] = strings.TrimSpace(ui.Plain(v))
				}
			}
		}
		for _, k := range []string{"instance", "servicename", "job"} {
			if v := labels[i][k]; v != "" {
				names[i] = v
				break
			}
		}
		if names[i] == "" {
			if keys := graphLabelKeys(labels, []int{i}); keys[0] != "__name__" {
				names[i] = ui.Plain(keys[0]) + "=" + labels[i][keys[0]]
			} else if name := strings.TrimSpace(ui.Plain(s.Name)); name != "" {
				names[i] = name
			} else {
				names[i] = "series"
			}
		}
		groups[names[i]] = append(groups[names[i]], i)
	}
	for _, group := range groups {
		if len(group) > 1 {
			distinguishGraphNames(names, labels, group, graphLabelKeys(labels, group))
		}
	}
	// Labels themselves can contain separators, or two label sets can be
	// identical. Add stable numeric suffixes without taking another name.
	reserved := make(map[string]bool, len(names))
	for _, name := range names {
		reserved[name] = true
	}
	seen := make(map[string]int, len(names))
	for i, name := range names {
		seen[name]++
		if seen[name] == 1 {
			continue
		}
		for n := seen[name]; ; n++ {
			candidate := fmt.Sprintf("%s #%d", name, n)
			if !reserved[candidate] {
				names[i], reserved[candidate], seen[name] = candidate, true, n
				break
			}
		}
	}
	return names
}

func graphLabelKeys(labels []map[string]string, group []int) []string {
	var keys []string
	seen := make(map[string]bool)
	for _, i := range group {
		for k, v := range labels[i] {
			if v != "" && !seen[k] {
				switch k {
				case "mountpoint", "device", "__name__":
				default:
					keys = append(keys, k)
				}
				seen[k] = true
			}
		}
	}
	slices.Sort(keys)
	if seen["device"] {
		keys = append([]string{"device"}, keys...)
	}
	if seen["mountpoint"] {
		keys = append([]string{"mountpoint"}, keys...)
	}
	return append(keys, "__name__")
}

func distinguishGraphNames(names []string, labels []map[string]string, group []int, keys []string) {
	for pos, k := range keys {
		first := labels[group[0]][k]
		if !slices.ContainsFunc(group, func(i int) bool { return labels[i][k] != first }) {
			continue
		}
		parts := make(map[string][]int)
		for _, i := range group {
			v := labels[i][k]
			if v == "" {
				names[i] += " · " + ui.Plain(k) + "=—"
			} else {
				names[i] += " · " + v
			}
			parts[v] = append(parts[v], i)
		}
		for _, part := range parts {
			if len(part) > 1 {
				distinguishGraphNames(names, labels, part, keys[pos+1:])
			}
		}
		return
	}
}
