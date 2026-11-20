package app

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"promtui/internal/config"
)

func TestConnectionTLS(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics/api/v1/query":
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		case "/alerts/api/v2/alerts":
			fmt.Fprint(w, `[]`)
		case "/agent-a/api/v1/targets", "/agent-b/api/v1/targets":
			fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[]}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()

	for _, tc := range []struct{ name, global string }{
		{"default", ""},
		{"enabled", "insecure_skip_verify: true\n"},
		{"disabled", "insecure_skip_verify: false\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.global + "connections:\n"
			for _, conn := range []struct{ name, override string }{
				{"inherit", ""},
				{"skip", "    insecure_skip_verify: true\n"},
				{"verify", "    insecure_skip_verify: false\n"},
			} {
				body += fmt.Sprintf("  %s:\n    url: %s/metrics\n    alertmanager_url: %s/alerts\n%s",
					conn.name, server.URL, server.URL, conn.override)
				body += fmt.Sprintf("    targets:\n      - {name: a, url: %s/agent-a}\n      - {name: b, url: %s/agent-b}\n", server.URL, server.URL)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path, false)
			if err != nil {
				t.Fatal(err)
			}
			m := New(Options{Connection: cfg.Connections[0], Connections: cfg.Connections, Timeout: 3 * time.Second, Limit: 10}, nil)
			for i, want := range []bool{tc.name == "enabled", true, false} {
				m.activeTab = promQLTab
				m.connectionRow = i
				m.connectionKey("enter")
				m.editor.SetValue("up")
				queryErr := m.run()().(completed).err
				m.activeTab = alertsTab
				alertsErr := m.refreshAlerts()().(alertsCompleted).err
				if want && (queryErr != nil || alertsErr != nil) {
					t.Errorf("%s should accept the certificate: query = %v, alerts = %v", m.opt.Connection.Name, queryErr, alertsErr)
				}
				var untrusted x509.UnknownAuthorityError
				if !want && (!errors.As(queryErr, &untrusted) || alertsErr == nil) {
					t.Errorf("%s should reject the certificate: query = %v, alerts = %v", m.opt.Connection.Name, queryErr, alertsErr)
				}
				m.activeTab = targetsTab
				for _, cmd := range targetCommands(m.refreshTargets()) {
					msg := cmd().(targetsCompleted)
					if want && msg.err != nil || !want && !errors.As(msg.err, &untrusted) {
						t.Errorf("%s source %d TLS error = %v (accept = %v)", m.opt.Connection.Name, msg.source, msg.err, want)
					}
					m.onTargetsCompleted(msg, time.Now())
				}
			}
		})
	}
}
