package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

const MetadataLimit = 80

// Suggestions uses only label APIs. Prefix matchers narrow results before
// the backend applies its limit (important on high-cardinality instances).
func (c Client) Suggestions(ctx context.Context, kind, metric, label, prefix string) ([]string, error) {
	path := "/labels"
	form := url.Values{"limit": {strconv.Itoa(MetadataLimit)}}
	if metric != "" && !metricName.MatchString(metric) {
		return nil, nil
	}
	switch kind {
	case "expr":
		if prefix == "" {
			return nil, nil
		}
		path = "/label/__name__/values"
		form.Set("match[]", `{__name__=~`+strconv.Quote(regexp.QuoteMeta(prefix)+".*")+"}")
	case "value":
		if !labelName.MatchString(label) || metric == "" && prefix == "" {
			return nil, nil
		}
		path = "/label/" + label + "/values"
		selector := metric
		if prefix != "" {
			selector += "{" + label + "=~" + strconv.Quote(regexp.QuoteMeta(prefix)+".*") + "}"
		}
		form.Set("match[]", selector)
	case "label":
		if metric != "" {
			form.Set("match[]", metric)
		}
	default:
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path+"?"+form.Encode(), nil)
	if err != nil {
		return nil, err
	}
	r, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	const bound = 256 << 10
	buf, err := io.ReadAll(io.LimitReader(r.Body, bound+1))
	if err != nil {
		return nil, err
	}
	if len(buf) > bound {
		return nil, fmt.Errorf("metadata response exceeds 256 KiB")
	}
	var env struct {
		Status, Error string
		Data          []string
	}
	if err = json.Unmarshal(buf, &env); err != nil {
		return nil, fmt.Errorf("metadata HTTP %s: %w", r.Status, err)
	}
	if r.StatusCode/100 != 2 || env.Status != "success" {
		return nil, fmt.Errorf("metadata HTTP %s: %s", r.Status, env.Error)
	}
	return env.Data[:min(MetadataLimit, len(env.Data))], nil
}
