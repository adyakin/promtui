package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type VMAlert struct {
	ID         string
	RuleID     string `json:"rule_id"`
	GroupID    string `json:"group_id"`
	Expression string
	ActiveAt   time.Time
	Labels     map[string]string
}

type VMAlertRef struct {
	GroupID, AlertID string
}

// ResolveVMAlert matches the generator route to configured API bases. The
// caller must use the returned index, not the generator origin, for requests.
func ResolveVMAlert(raw string, bases []string) (VMAlertRef, int, error) {
	u, err := url.Parse(raw)
	if err != nil || !httpOrigin(u) || !strings.HasSuffix(u.Path, "/alert") {
		return VMAlertRef{}, -1, errors.New("Invalid vmalert generator URL")
	}
	values, err := url.ParseQuery(u.RawQuery)
	group, alert := values["group_id"], values["alert_id"]
	if err != nil || len(group) != 1 || len(alert) != 1 || !vmID(group[0]) || !vmID(alert[0]) {
		return VMAlertRef{}, -1, errors.New("Invalid vmalert generator IDs")
	}
	ref := VMAlertRef{GroupID: group[0], AlertID: alert[0]}
	path := strings.TrimSuffix(u.EscapedPath(), "/alert")
	best, rank, ambiguous := -1, 0, false
	for i, base := range bases {
		b, err := url.Parse(base)
		if err != nil || !httpOrigin(b) || u.Scheme != b.Scheme ||
			!strings.EqualFold(u.Hostname(), b.Hostname()) || originPort(u) != originPort(b) {
			continue
		}
		prefix := strings.TrimSuffix(strings.TrimRight(b.EscapedPath(), "/"), "/api/v1")
		score := 0
		switch {
		case path == prefix:
			score = 2
		case path == prefix+"/vmalert":
			// vmalert serves both API routes, with and without /vmalert.
			score = 1
		}
		if score > rank {
			best, rank, ambiguous = i, score, false
		} else if score > 0 && score == rank {
			ambiguous = true
		}
	}
	switch {
	case ambiguous:
		return ref, -1, errors.New("Ambiguous configured vmalert source")
	case best < 0:
		return ref, -1, errors.New("Generator does not match a configured vmalert source")
	}
	return ref, best, nil
}

func httpOrigin(u *url.URL) bool {
	return u != nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func originPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func vmID(s string) bool {
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

// VMAlert fetches a single alert from vmalert only when requested by a user
// action. The endpoint returns the alert object directly, not a data envelope.
func (c Client) VMAlert(ctx context.Context, ref VMAlertRef) (VMAlert, error) {
	form := url.Values{"group_id": {ref.GroupID}, "alert_id": {ref.AlertID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/alert?"+form.Encode(), nil)
	if err != nil || !httpOrigin(req.URL) {
		return VMAlert{}, errors.New("Invalid vmalert API URL")
	}
	req.Header.Set("Accept", "application/json")
	client := *c.HTTP
	// Never follow a redirect to a destination other than the configured API.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return VMAlert{}, vmAlertRequestError(ctx)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone {
		return VMAlert{}, errors.New("Alert no longer available in vmalert")
	}
	if res.StatusCode/100 != 2 {
		return VMAlert{}, fmt.Errorf("vmalert Alert API returned HTTP %d", res.StatusCode)
	}
	const bound = 256 << 10
	body, err := io.ReadAll(io.LimitReader(res.Body, bound+1))
	if err != nil {
		return VMAlert{}, vmAlertRequestError(ctx)
	}
	if len(body) > bound {
		return VMAlert{}, errors.New("vmalert Alert API response exceeds 256 KiB")
	}
	var out VMAlert
	if json.Unmarshal(body, &out) != nil {
		return VMAlert{}, errors.New("Invalid vmalert alert response")
	}
	if out.ID != ref.AlertID || out.GroupID != ref.GroupID || !vmID(out.RuleID) {
		return VMAlert{}, errors.New("Missing or mismatched vmalert alert IDs")
	}
	return out, nil
}

func vmAlertRequestError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("vmalert alert request: %w", err)
	}
	return errors.New("vmalert Alert API connection failed")
}
