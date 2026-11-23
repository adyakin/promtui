package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestYAMLTableViews(t *testing.T) {
	cfg, err := decode("config.yaml", []byte(`connections:
  prod:
    url: https://query.test
    alerts:
      columns:
        - state
        - {field: labels.app.service, title: 'My Service'}
    vmagent:
      - {name: vmagent, url: 'https://agent.test'}
    targets_view:
      search: [labels.hidden, scrape_url]
  staging:
    url: https://prom.test
    alerts:
      search: [labels.alertname]
    targets_view:
      columns: [health, {field: labels.servicename}, instance, last_scrape]
      search: [source]
  plain:
    url: https://plain.test
`))
	if err != nil {
		t.Fatal(err)
	}
	prod, staging, plain := cfg.Connections[0], cfg.Connections[1], cfg.Connections[2]
	wantProdAlerts := TableView{Columns: []Column{{Field: "state"}, {Field: "labels.app.service", Title: "My Service"}}}
	wantProdTargets := TableView{Search: []string{"labels.hidden", "scrape_url"}}
	wantStagingTargets := TableView{
		Columns: []Column{{Field: "health"}, {Field: "labels.servicename"}, {Field: "instance"}, {Field: "last_scrape"}},
		Search:  []string{"source"},
	}
	if !reflect.DeepEqual(prod.Alerts, wantProdAlerts) || !reflect.DeepEqual(prod.TargetsView, wantProdTargets) ||
		!reflect.DeepEqual(staging.Alerts, TableView{Search: []string{"labels.alertname"}}) ||
		!reflect.DeepEqual(staging.TargetsView, wantStagingTargets) || len(prod.Targets) != 1 ||
		!reflect.DeepEqual(plain.Alerts, TableView{}) || !reflect.DeepEqual(plain.TargetsView, TableView{}) {
		t.Fatalf("unexpected views: %+v", cfg.Connections)
	}
}

func TestYAMLTableViewValidation(t *testing.T) {
	for _, tt := range []struct{ yaml, want string }{
		{"alerts: {columns: []}", "alerts.columns: expected a nonempty list"},
		{"alerts: {search: []}", "alerts.search: expected a nonempty list"},
		{"targets_view: {columns: null}", "targets_view.columns: expected a nonempty list"},
		{"targets_view: {search: []}", "targets_view.search: expected a nonempty list"},
		{"alerts: {columns: [state, health]}", `alerts.columns[1]: unknown field "health"`},
		{"targets_view: {search: [state]}", `targets_view.search[0]: unknown field "state"`},
		{"targets_view: {columns: [discoveredLabels.__address__]}", "targets_view.columns[0]: unknown field"},
		{"alerts: {search: [labels.]}", "alerts.search[0]: label reference"},
		{"alerts: {columns: ['', state]}", "alerts.columns[0]: field must not be empty"},
		{"alerts: {columns: [{title: Service}]}", "alerts.columns[0]: missing field"},
		{"alerts: {columns: [{field: state, title: '  '}]}", "alerts.columns[0]: title must not be empty"},
		{"targets_view: {columns: [{field: health, title: null}]}", "targets_view.columns[0]: title must be a string"},
		{"targets_view: {columns: [{field: health, width: 9}]}", "targets_view.columns[0]: unknown column property"},
		{"targets_view: {columns: [health, {field: health}]}", `targets_view.columns[1]: duplicate column "health"`},
		{"targets_view: {search: [false]}", "targets_view.search[0]: field must be a string"},
		{"targets_view: {columns: [health], columns: [source]}", "targets_view.columns: duplicate property"},
		{"alerts: {visible: [state]}", "alerts: unknown property"},
		{"alerts: false", "alerts: expected a mapping"},
	} {
		t.Run(tt.yaml, func(t *testing.T) {
			data := "connections:\n  prod:\n    url: https://query.test\n    " + tt.yaml + "\n"
			_, err := decode("config.yaml", []byte(data))
			if err == nil || !strings.Contains(err.Error(), `connection "prod": `+tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
