package alertmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
)

var ErrSilenceNotFound = errors.New("silence no longer exists (HTTP 404); refresh the list")

type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

// Alertmanager v2 permits older responses without isEqual; those are positive.
func (m *Matcher) UnmarshalJSON(data []byte) error {
	type wire Matcher
	v := wire{IsEqual: true}
	err := json.Unmarshal(data, &v)
	*m = Matcher(v)
	return err
}

func (m Matcher) Operator() string {
	switch {
	case m.IsRegex && m.IsEqual:
		return "=~"
	case m.IsRegex:
		return "!~"
	case m.IsEqual:
		return "="
	default:
		return "!="
	}
}

func (m Matcher) String() string {
	name := m.Name
	if !model.LabelName(name).IsValidLegacy() {
		name = strconv.Quote(name)
	}
	return name + m.Operator() + strconv.Quote(m.Value)
}

type SilenceSpec struct {
	ID          string            `json:"id,omitempty"`
	Matchers    []Matcher         `json:"matchers"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	CreatedBy   string            `json:"createdBy"`
	Comment     string            `json:"comment"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Silence struct {
	SilenceSpec
	Status struct {
		State string `json:"state"`
	} `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s Silence) State(now time.Time) string {
	switch {
	case s.Status.State == "expired" || !s.EndsAt.After(now):
		return "expired"
	case s.StartsAt.After(now):
		return "pending"
	default:
		return "active"
	}
}

func (s Silence) valid() bool {
	return s.ID != "" && !s.StartsAt.IsZero() && !s.EndsAt.IsZero() && !s.UpdatedAt.IsZero() && len(s.Matchers) > 0 &&
		(s.Status.State == "active" || s.Status.State == "pending" || s.Status.State == "expired")
}

func (c Client) Silences(ctx context.Context) ([]Silence, error) {
	var rows []Silence
	if err := c.request(ctx, http.MethodGet, "api/v2/silences", "", nil, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("invalid Alertmanager response: expected a JSON array of silences")
	}
	for _, row := range rows {
		if !row.valid() {
			return nil, errors.New("invalid or unsupported silence in Alertmanager response")
		}
	}
	return rows, nil
}

func silencePath(id string) (string, error) {
	if id == "" || id == "." || id == ".." {
		return "", errors.New("invalid silence ID")
	}
	return "api/v2/silence/" + url.PathEscape(id), nil
}

func (c Client) Silence(ctx context.Context, id string) (Silence, error) {
	var row Silence
	path, err := silencePath(id)
	if err == nil {
		err = c.request(ctx, http.MethodGet, path, "", nil, &row)
	}
	if err == nil && (!row.valid() || row.ID != id) {
		err = errors.New("invalid or unsupported silence in Alertmanager response")
	}
	return row, err
}

func (c Client) SaveSilence(ctx context.Context, spec SilenceSpec) (string, error) {
	var result struct {
		ID string `json:"silenceID"`
	}
	err := c.request(ctx, http.MethodPost, "api/v2/silences", "", spec, &result)
	if err == nil && result.ID == "" {
		err = errors.New("Alertmanager did not return a silence ID; refresh before retrying")
	}
	return result.ID, err
}

func (c Client) ExpireSilence(ctx context.Context, id string) error {
	path, err := silencePath(id)
	if err != nil {
		return err
	}
	return c.request(ctx, http.MethodDelete, path, "", nil, nil)
}

// CompileMatchers follows Alertmanager's pkg/labels.NewMatcher: Go regexp,
// wrapped in ^(?:…)$ without changing the user's flags or newline semantics.
// PromQL's matcher wraps regex in a dot-all group, so it isn't a drop-in here.
func CompileMatchers(matchers []Matcher) (matches func(map[string]string) bool, broad bool, err error) {
	if len(matchers) == 0 {
		return nil, false, errors.New("add at least one matcher")
	}
	checks := make([]func(string) bool, 0, len(matchers))
	positive := false
	for i, m := range matchers {
		if !model.UTF8Validation.IsValidLabelName(m.Name) {
			return nil, false, fmt.Errorf("matcher %d: enter a valid, non-empty label name", i+1)
		}
		test := func(v string) bool { return v == m.Value }
		if m.IsRegex {
			if _, e := regexp.Compile(m.Value); e != nil {
				return nil, false, fmt.Errorf("matcher %d: %w", i+1, e)
			}
			re, e := regexp.Compile("^(?:" + m.Value + ")$")
			if e != nil {
				return nil, false, fmt.Errorf("matcher %d: %w", i+1, e)
			}
			test = re.MatchString
		}
		if !m.IsEqual {
			negated := test
			test = func(v string) bool { return !negated(v) }
		}
		checks = append(checks, test)
		positive = positive || m.IsEqual
		broad = broad || test("")
	}
	return func(labels map[string]string) bool {
		for i, m := range matchers {
			if !checks[i](labels[m.Name]) {
				return false
			}
		}
		return true
	}, broad || !positive, nil
}
