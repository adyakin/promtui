package app

import (
	"context"
	"crypto/tls"
	"net/http"

	"promtui/internal/alertmanager"
)

func (m *Model) alertmanagerRequest() (alertmanager.Client, context.Context, context.CancelFunc) {
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = alertRefreshInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return alertmanager.Client{Base: m.opt.Connection.AlertmanagerURL, HTTP: m.connectionHTTP()}, ctx, cancel
}

// connectionHTTP is called before launching a request so it captures the
// selected connection even if the user switches while the request is running.
func (m *Model) connectionHTTP() *http.Client {
	skipVerify := m.opt.Connection.InsecureSkipVerify
	if client := m.httpClients[skipVerify]; client != nil {
		return client
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = skipVerify
	client := &http.Client{Transport: transport}
	if m.httpClients == nil {
		m.httpClients = make(map[bool]*http.Client)
	}
	m.httpClients[skipVerify] = client
	return client
}
