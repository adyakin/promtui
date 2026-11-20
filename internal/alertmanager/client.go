// Package alertmanager owns the Alertmanager v2 HTTP API and wire format.
package alertmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Alert struct {
	Fingerprint                 string
	Labels, Annotations         map[string]string
	StartsAt, EndsAt, UpdatedAt time.Time
	GeneratorURL                string
	Receivers                   []struct{ Name string }
	Status                      struct {
		State                   string
		SilencedBy, InhibitedBy []string
	}
}

type Client struct {
	Base string
	HTTP *http.Client
}

var ErrConfig = errors.New("set alertmanager_url to an HTTP(S) base URL without credentials, query or fragment")
var ErrAuth = errors.New("Alertmanager authentication or authorization failed")

func (c Client) Alerts(ctx context.Context) ([]Alert, error) {
	base, err := url.Parse(c.Base)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") ||
		base.User != nil || base.ForceQuery || base.RawQuery != "" || strings.Contains(c.Base, "#") {
		return nil, ErrConfig
	}
	endpoint := base.JoinPath("api/v2/alerts")
	endpoint.RawQuery = "active=true&silenced=true&inhibited=true&unprocessed=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrConfig
	}
	req.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, requestError(ctx, err, "could not reach Alertmanager (check connectivity and TLS)")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w (HTTP %d)", ErrAuth, res.StatusCode)
	case res.StatusCode/100 != 2:
		return nil, fmt.Errorf("Alertmanager returned HTTP %d", res.StatusCode)
	}

	const maxBody = 32 << 20
	buf, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return nil, requestError(ctx, err, "could not read Alertmanager response")
	}
	if len(buf) > maxBody {
		return nil, errors.New("Alertmanager response exceeds 32 MiB")
	}
	var alerts []Alert
	if err := json.Unmarshal(buf, &alerts); err != nil || alerts == nil {
		return nil, errors.New("invalid Alertmanager response: expected a JSON array of alerts")
	}
	return alerts, nil
}

// Do not expose raw transport errors, response bodies or URLs in the UI.
func requestError(ctx context.Context, err error, fallback string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return context.DeadlineExceeded
	}
	return errors.New(fallback)
}
