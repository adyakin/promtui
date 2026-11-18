// Package api owns the Prometheus HTTP envelope and wire formats.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/prometheus/common/model"
)

type Response struct {
	Value    model.Value
	Warnings []string
}

type Client struct {
	Base string
	HTTP *http.Client
}

func (c Client) Query(ctx context.Context, ranged bool, form url.Values) (Response, error) {
	base := strings.TrimRight(c.Base, "/")
	if !strings.HasSuffix(base, "/api/v1") {
		base += "/api/v1"
	}
	path := "/query"
	if ranged {
		path = "/query_range"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	const maxBody = 32 << 20
	buf, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return Response{}, err
	}
	if len(buf) > maxBody {
		return Response{}, fmt.Errorf("response exceeds 32 MiB; narrow the query or time range")
	}
	var env struct {
		Status, ErrorType, Error string
		Warnings, Infos          []string
		IsPartial                bool
		Data                     struct {
			ResultType string
			Result     json.RawMessage
		}
	}
	if err = json.Unmarshal(buf, &env); err != nil {
		return Response{}, fmt.Errorf("HTTP %s: invalid Prometheus response: %w", res.Status, err)
	}
	if res.StatusCode/100 != 2 || env.Status != "success" {
		return Response{}, fmt.Errorf("HTTP %s: %s: %s", res.Status, env.ErrorType, env.Error)
	}
	out := Response{Warnings: append(env.Warnings, env.Infos...)}
	if env.IsPartial {
		out.Warnings = append(out.Warnings, "backend returned partial results")
	}
	switch env.Data.ResultType {
	case "vector":
		out.Value = new(model.Vector)
	case "matrix":
		out.Value = new(model.Matrix)
	case "scalar":
		out.Value = new(model.Scalar)
	case "string":
		out.Value = new(model.String)
	default:
		return Response{}, fmt.Errorf("unsupported result type %q", env.Data.ResultType)
	}
	if string(env.Data.Result) == "null" {
		return Response{}, fmt.Errorf("missing query result")
	}
	err = json.Unmarshal(env.Data.Result, out.Value)
	return out, err
}
