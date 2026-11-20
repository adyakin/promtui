package app

import (
	"crypto/tls"
	"net/http"

	"promtui/internal/config"
)

// connectionHTTP is called before launching a request so it captures the
// selected connection even if the user switches while the request is running.
func (m *Model) connectionHTTP() *http.Client {
	conn := m.opt.Connection
	if client := m.httpClients[conn]; client != nil {
		return client
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = conn.InsecureSkipVerify
	client := &http.Client{Transport: transport}
	if m.httpClients == nil {
		m.httpClients = make(map[config.Connection]*http.Client)
	}
	m.httpClients[conn] = client
	return client
}
