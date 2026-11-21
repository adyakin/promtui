package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	AlertingRule  = "alerting"
	RecordingRule = "recording"
)

type RuleEvaluation struct {
	LastEvaluation time.Time
	EvaluationTime *float64 // seconds
}

type RuleGroup struct {
	Name, File, ID, Type string
	RuleEvaluation
	Interval, EvalOffset, EvalDelay *float64 // seconds
	Limit, Concurrency              *int
	Labels                          map[string]string
	Rules                           []Rule
}

type Rule struct {
	Name, ID, Type, State, Health, LastError string
	Expression                               string
	Position                                 int // original position in the group
	RuleEvaluation
	For, KeepFiringFor             *float64 // seconds
	Labels, Annotations            map[string]string
	Alerts                         []RuleAlert
	LastSamples, LastSeriesFetched *int
}

type RuleAlert struct {
	ID, Name, State, Value    string
	Labels, Annotations       map[string]string
	ActiveAt, KeepFiringSince time.Time
	Restored, Stabilizing     *bool
}

type Rules struct {
	Groups  []RuleGroup
	Warning string
}

// Rules fetches the same endpoint for Prometheus and vmalert. Don't surface
// raw transport errors or API error bodies: they can contain sensitive URLs.
func (c Client) Rules(ctx context.Context) (Rules, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/rules", nil)
	if err != nil || req.URL.Hostname() == "" || req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return Rules{}, errors.New("invalid Rules API URL")
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Rules{}, rulesRequestError(err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return Rules{}, fmt.Errorf("Rules API returned HTTP %d", res.StatusCode)
	}
	const maxBody = 32 << 20
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return Rules{}, rulesRequestError(err)
	}
	if len(body) > maxBody {
		return Rules{}, errors.New("Rules API response exceeds 32 MiB")
	}
	return decodeRules(body)
}

func rulesRequestError(err error) error {
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("Rules request cancelled")
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return errors.New("Rules request timed out")
	default:
		return errors.New("Rules API connection failed")
	}
}

// See Prometheus web/api/v1/api.go and vmalert app/vmalert/rule/web.go.
// Keep group/rule records separate so a bad optional field or child needn't
// discard an otherwise readable response.
func decodeRules(body []byte) (Rules, error) {
	var env struct {
		Status string
		Data   struct{ Groups []json.RawMessage }
	}
	if json.Unmarshal(body, &env) != nil {
		return Rules{}, errors.New("invalid Rules API JSON response")
	}
	if env.Status != "success" {
		return Rules{}, errors.New("Rules API reported an unsuccessful response")
	}
	if env.Data.Groups == nil {
		return Rules{}, errors.New("Rules API response is missing rule groups")
	}
	var out Rules
	var malformed int
	for _, raw := range env.Data.Groups {
		var group struct {
			RuleGroup
			LastEvaluation string
			EvalOffset     *float64 `json:"eval_offset"`
			EvalDelay      *float64 `json:"eval_delay"`
			Rules          []json.RawMessage
		}
		if json.Unmarshal(raw, &group) != nil || group.Name == "" || group.Rules == nil || invalidRuleTime(group.LastEvaluation) {
			malformed++
		}
		if group.Name == "" {
			continue
		}
		g := group.RuleGroup
		g.LastEvaluation = ruleTime(group.LastEvaluation)
		g.EvalOffset, g.EvalDelay = group.EvalOffset, group.EvalDelay
		for pos, raw := range group.Rules {
			var wire struct {
				Rule
				Query, LastEvaluation string
				Duration              *float64
				SnakeKeepFiringFor    *float64 `json:"keep_firing_for"`
				Alerts                []json.RawMessage
			}
			bad := json.Unmarshal(raw, &wire) != nil || invalidRuleTime(wire.LastEvaluation)
			if bad || wire.Name == "" {
				malformed++
			}
			if wire.Name == "" {
				continue
			}
			r := wire.Rule
			r.Position, r.Expression, r.For = pos, wire.Query, wire.Duration
			r.LastEvaluation = ruleTime(wire.LastEvaluation)
			r.LastError = ruleAuthHeader.ReplaceAllString(ruleErrorURL.ReplaceAllString(r.LastError, "[URL redacted]"), "$1[redacted]")
			r.Type, r.State = strings.ToLower(r.Type), strings.ToLower(r.State)
			if r.KeepFiringFor == nil {
				r.KeepFiringFor = wire.SnakeKeepFiringFor
			}
			if r.Type == AlertingRule {
				for _, raw := range wire.Alerts {
					var alert struct {
						RuleAlert
						ActiveAt, KeepFiringSince string
					}
					bad := json.Unmarshal(raw, &alert) != nil || invalidRuleTime(alert.ActiveAt) || invalidRuleTime(alert.KeepFiringSince)
					usable := alert.State != "" || len(alert.Labels) > 0 || alert.ID != "" || alert.Name != ""
					if bad || !usable {
						malformed++
					}
					if usable {
						a := alert.RuleAlert
						a.State = strings.ToLower(a.State)
						a.ActiveAt, a.KeepFiringSince = ruleTime(alert.ActiveAt), ruleTime(alert.KeepFiringSince)
						r.Alerts = append(r.Alerts, a)
					}
				}
			}
			g.Rules = append(g.Rules, r)
		}
		out.Groups = append(out.Groups, g)
	}
	if len(env.Data.Groups) > 0 && len(out.Groups) == 0 {
		return Rules{}, errors.New("Rules API response contains no readable rule groups")
	}
	if malformed > 0 {
		out.Warning = "Some rule data was malformed or missing"
	}
	return out, nil
}

var (
	ruleErrorURL   = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	ruleAuthHeader = regexp.MustCompile(`(?i)\b((?:proxy-)?authorization["']?\s*[:=]\s*)[^\r\n]+`)
)

func ruleTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func invalidRuleTime(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, s)
	return err != nil
}
