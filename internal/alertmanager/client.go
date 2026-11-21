// Package alertmanager owns the Alertmanager v2 HTTP API and wire format.
package alertmanager

import (
	"bytes"
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
	var alerts []Alert
	err := c.request(ctx, http.MethodGet, "api/v2/alerts", "active=true&silenced=true&inhibited=true&unprocessed=true", nil, &alerts)
	if err == nil && alerts == nil {
		err = errors.New("invalid Alertmanager response: expected a JSON array of alerts")
	}
	return alerts, err
}

func (c Client) request(ctx context.Context, method, path, query string, data, result any) error {
	base, err := url.Parse(c.Base)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") ||
		base.User != nil || base.ForceQuery || base.RawQuery != "" || strings.Contains(c.Base, "#") {
		return ErrConfig
	}
	endpoint := base.JoinPath(path)
	endpoint.RawQuery = query
	var body io.Reader
	if data != nil {
		buf, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("encode Alertmanager request: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return ErrConfig
	}
	req.Header.Set("Accept", "application/json")
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	if method != http.MethodGet {
		// Do not silently retry or forward a write to a different endpoint.
		direct := *client
		direct.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &direct
	}
	res, err := client.Do(req)
	if err != nil {
		return requestError(ctx, err, "could not reach Alertmanager (check connectivity and TLS)")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w (HTTP %d)", ErrAuth, res.StatusCode)
	case res.StatusCode == http.StatusNotFound && strings.HasPrefix(path, "api/v2/silence/"):
		return ErrSilenceNotFound
	case res.StatusCode/100 != 2:
		if res.StatusCode == http.StatusNotFound && method == http.MethodPost {
			return fmt.Errorf("Alertmanager returned HTTP 404 (silence no longer exists or endpoint unavailable)")
		}
		// Alertmanager's v2 silence handlers send JSON strings for validation errors.
		// Never display arbitrary proxy HTML or unbounded response bodies.
		if res.StatusCode == http.StatusBadRequest && strings.HasPrefix(path, "api/v2/silence") {
			buf, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			var reason string
			if json.Unmarshal(buf, &reason) == nil && strings.TrimSpace(reason) != "" {
				return fmt.Errorf("Alertmanager rejected the silence (HTTP 400): %q", reason)
			}
			return errors.New("Alertmanager rejected the silence (HTTP 400); check the matchers and timestamps")
		}
		return fmt.Errorf("Alertmanager returned HTTP %d", res.StatusCode)
	}
	if result == nil {
		return nil
	}

	const maxBody = 32 << 20
	buf, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return requestError(ctx, err, "could not read Alertmanager response")
	}
	if len(buf) > maxBody {
		return errors.New("Alertmanager response exceeds 32 MiB")
	}
	if err := json.Unmarshal(buf, result); err != nil {
		return errors.New("invalid Alertmanager response: malformed JSON")
	}
	return nil
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
