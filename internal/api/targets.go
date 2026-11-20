package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var (
	ErrTargetsUnsupported = errors.New("unsupported Targets API endpoint")
	ErrTargetsAuth        = errors.New("Targets API authentication or authorization failed")
)

type Target struct {
	DiscoveredLabels   map[string]string `json:"discoveredLabels"`
	Labels             map[string]string `json:"labels"`
	ScrapePool         string            `json:"scrapePool"`
	ScrapeURL          string            `json:"scrapeUrl"`
	GlobalURL          string            `json:"globalUrl"`
	LastError          string            `json:"lastError"`
	LastScrape         string            `json:"lastScrape"`
	LastScrapeDuration *float64          `json:"lastScrapeDuration"`
	Health             string            `json:"health"`
	ScrapeInterval     string            `json:"scrapeInterval"`
	ScrapeTimeout      string            `json:"scrapeTimeout"`
}

type Targets struct {
	Active  []Target `json:"activeTargets"`
	Dropped []Target `json:"droppedTargets"`
}

func (c Client) Targets(ctx context.Context) (Targets, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/targets?state=any", nil)
	if err != nil {
		return Targets{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Targets{}, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return Targets{}, fmt.Errorf("%w (HTTP %s)", ErrTargetsAuth, res.Status)
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return Targets{}, fmt.Errorf("%w (HTTP %s)", ErrTargetsUnsupported, res.Status)
	}
	if res.StatusCode/100 != 2 {
		return Targets{}, fmt.Errorf("Targets API returned HTTP %s", res.Status)
	}
	const maxBody = 32 << 20
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return Targets{}, err
	}
	if len(body) > maxBody {
		return Targets{}, errors.New("Targets API response exceeds 32 MiB")
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return Targets{}, fmt.Errorf("%w (expected JSON, received markup)", ErrTargetsUnsupported)
	}
	var env struct {
		Status, Error string
		Data          *Targets
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return Targets{}, fmt.Errorf("invalid Targets API response: %w", err)
	}
	if env.Status != "success" || env.Data == nil {
		if env.Error != "" {
			return Targets{}, fmt.Errorf("Targets API: %s", env.Error)
		}
		return Targets{}, errors.New("invalid Targets API response: missing successful target data")
	}
	return *env.Data, nil
}
