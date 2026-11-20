package app

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"promtui/internal/alertmanager"
)

type alertStatus uint8

const (
	alertActive alertStatus = iota
	alertSilenced
	alertInhibited
	alertUnprocessed
	alertUnknown
)

type alertRow struct {
	alertmanager.Alert
	id     string
	group  alertStatus
	search []string
}

func newAlertRow(source string, a alertmanager.Alert) alertRow {
	id := source + "\x00fingerprint:" + a.Fingerprint
	if a.Fingerprint == "" {
		labels := a.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		// JSON sorts map keys, and escaping preserves label boundaries.
		key, _ := json.Marshal(labels)
		id = source + "\x00labels:" + string(key)
	}
	group := alertUnknown
	switch {
	case len(a.Status.SilencedBy) > 0:
		group = alertSilenced
	case len(a.Status.InhibitedBy) > 0:
		group = alertInhibited
	case a.Status.State == "active":
		group = alertActive
	case a.Status.State == "unprocessed":
		group = alertUnprocessed
	}
	row := alertRow{Alert: a, id: id, group: group}
	row.search = []string{source, a.Fingerprint, row.state(), a.Status.State, a.GeneratorURL}
	for _, timestamp := range []time.Time{a.StartsAt, a.EndsAt, a.UpdatedAt} {
		if !timestamp.IsZero() {
			row.search = append(row.search, timestamp.Format(time.RFC3339Nano))
		}
	}
	for _, receiver := range a.Receivers {
		row.search = append(row.search, receiver.Name)
	}
	row.search = append(row.search, a.Status.SilencedBy...)
	row.search = append(row.search, a.Status.InhibitedBy...)
	return row
}

func (a alertRow) state() string {
	if a.group == alertSilenced && len(a.Status.InhibitedBy) > 0 {
		return "Silenced + Inhibited"
	}
	if a.group == alertUnknown && a.Status.State == "suppressed" {
		return "Suppressed"
	}
	return [...]string{"Active", "Silenced", "Inhibited", "Unprocessed", "Unknown"}[a.group]
}

type alertCounts struct{ total, active, silenced, inhibited int }

func countAlerts(rows []alertRow) alertCounts {
	c := alertCounts{total: len(rows)}
	for _, a := range rows {
		if a.group == alertActive {
			c.active++
		}
		if len(a.Status.SilencedBy) > 0 {
			c.silenced++
		}
		if len(a.Status.InhibitedBy) > 0 {
			c.inhibited++
		}
	}
	return c
}

func (a alertRow) matches(f fuzzyFilter) bool {
	return f.matches(a.search...) || matchLabels(f, a.Labels) || matchLabels(f, a.Annotations)
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	case "":
		return 4
	default:
		return 3
	}
}

func compareAlertRows(a, b alertRow, newestFirst bool) int {
	if !newestFirst {
		x, y := strings.ToLower(a.Labels["severity"]), strings.ToLower(b.Labels["severity"])
		if n := cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(severityRank(x), severityRank(y)), cmp.Compare(x, y)); n != 0 {
			return n
		}
	}
	if a.StartsAt.IsZero() != b.StartsAt.IsZero() {
		if a.StartsAt.IsZero() {
			return 1
		}
		return -1
	}
	if n := a.StartsAt.Compare(b.StartsAt); n != 0 {
		if newestFirst {
			return -n
		}
		return n
	}
	return cmp.Compare(a.id, b.id)
}

func alertDuration(since, now time.Time) string {
	if since.IsZero() {
		return "—"
	}
	seconds := int64(max(0, now.Sub(since)) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%dh%dm", seconds/3600, seconds/60%60)
	}
	return fmt.Sprintf("%dd%dh", seconds/86400, seconds/3600%24)
}

func alertLabelText(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		pairs = append(pairs, fmt.Sprintf("%s=%q", key, labels[key]))
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}
