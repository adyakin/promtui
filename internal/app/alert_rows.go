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
	"promtui/internal/config"
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
	id    string
	group alertStatus
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
	return alertRow{Alert: a, id: id, group: group}
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

var defaultAlertFields = [...]string{"state", "severity", "alertname", "instance", "duration"}

func (a alertRow) field(field string, now time.Time) string {
	if label, ok := config.LabelName(field); ok {
		return a.Labels[label]
	}
	switch field {
	case "state":
		return a.state()
	case "severity", "alertname", "instance":
		return a.Labels[field]
	case "duration":
		if !a.StartsAt.IsZero() {
			return alertDuration(a.StartsAt, now)
		}
	}
	return ""
}

func compareAlertField(a, b alertRow, field string, now time.Time) int {
	switch field {
	case "state":
		return cmp.Compare(a.group, b.group)
	case "severity":
		x, y := strings.ToLower(a.Labels["severity"]), strings.ToLower(b.Labels["severity"])
		return cmp.Or(cmp.Compare(severityRank(x), severityRank(y)), cmp.Compare(x, y))
	case "duration":
		return cmp.Or(missingLast(a.StartsAt.IsZero(), b.StartsAt.IsZero()),
			cmp.Compare(max(0, now.Sub(b.StartsAt)), max(0, now.Sub(a.StartsAt))))
	default:
		return compareFieldText(a.field(field, now), b.field(field, now))
	}
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
