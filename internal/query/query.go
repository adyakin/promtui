package query

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
	"promtui/internal/api"
)

type Request struct {
	Expression   string
	End          time.Time
	Window, Step time.Duration // zero window selects an instant query
	Limit        int
}

func (r Request) form() url.Values {
	f := url.Values{"query": {r.Expression}, "limit": {strconv.Itoa(r.Limit)}}
	end := r.End.UTC().Format(time.RFC3339Nano)
	if r.Window == 0 {
		f.Set("time", end)
		return f
	}
	step := r.Step
	if step == 0 {
		step = max(time.Second, (r.Window / 240).Round(time.Second))
	}
	f.Set("start", r.End.Add(-r.Window).UTC().Format(time.RFC3339Nano))
	f.Set("end", end)
	f.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
	return f
}

type Series struct {
	Name, Labels string
	Samples      []model.SamplePair
	Text         *string // string result instead of numeric sample
}
type Result struct {
	Series  []Series
	Notices []string
}

func Execute(ctx context.Context, c api.Client, r Request) (Result, error) {
	resp, err := c.Query(ctx, r.Window != 0, r.form())
	if err != nil {
		return Result{}, err
	}
	out := Result{Notices: resp.Warnings}
	skipped := false
	add := func(m model.Metric, samples []model.SamplePair, text *string) {
		name := string(m[model.MetricNameLabel])
		if name == "" {
			name = resp.Value.Type().String()
		}
		out.Series = append(out.Series, Series{name, m.String(), samples, text})
	}
	switch v := resp.Value.(type) {
	case *model.Vector:
		for _, s := range *v {
			if len(out.Series) >= r.Limit {
				break
			}
			if s == nil {
				return Result{}, fmt.Errorf("invalid null vector sample")
			}
			if s.Histogram != nil {
				skipped = true
				continue
			}
			add(s.Metric, []model.SamplePair{{Timestamp: s.Timestamp, Value: s.Value}}, nil)
		}
	case *model.Matrix:
		for _, s := range *v {
			if len(out.Series) >= r.Limit {
				break
			}
			if s == nil {
				return Result{}, fmt.Errorf("invalid null matrix series")
			}
			if len(s.Histograms) > 0 {
				skipped = true
			}
			if len(s.Values) > 0 {
				add(s.Metric, s.Values, nil)
			}
		}
	case *model.Scalar:
		add(nil, []model.SamplePair{{Timestamp: v.Timestamp, Value: v.Value}}, nil)
	case *model.String:
		add(nil, []model.SamplePair{{Timestamp: v.Timestamp}}, &v.Value)
	}
	if skipped {
		out.Notices = append(out.Notices, "native histogram samples omitted")
	}
	if len(out.Series) == r.Limit {
		out.Notices = append(out.Notices, fmt.Sprintf("series limit reached (%d)", r.Limit))
	}
	return out, ctx.Err()
}
