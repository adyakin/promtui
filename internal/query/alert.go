package query

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/VictoriaMetrics/metricsql"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"promtui/internal/api"
)

type AlertGraph struct {
	Expression, Notice string
}

const alertProbeLimit = 200

// BuildAlertGraph evaluates a bounded instant query to learn which alert
// labels really exist on the expression's output. In particular, it does
// not assume that alert labels are present on the inputs to an aggregation
// or on both sides of a vector operation.
func BuildAlertGraph(ctx context.Context, client api.Client, original string, labels map[string]string, at time.Time, allowMetricsQL bool) AlertGraph {
	expression, threshold, dialect, root := alertExpression(original, allowMetricsQL)
	out := AlertGraph{Expression: expression}
	var status []string
	if threshold {
		status = append(status, "Threshold removed")
	} else {
		status = append(status, "Threshold removal skipped")
	}
	// If the syntax is unknown, don't wrap or rewrite it. Let the backend
	// decide whether the original expression is valid.
	var scope []string
	if root != nil && len(labels) > 0 {
		form := url.Values{
			"query": {expression}, "time": {at.UTC().Format(time.RFC3339Nano)},
			"limit": {strconv.Itoa(alertProbeLimit)},
		}
		probe := func() (model.Vector, bool) {
			resp, err := client.Query(ctx, false, form)
			vector, ok := resp.Value.(*model.Vector)
			if err != nil || !ok {
				return nil, false
			}
			return *vector, len(resp.Warnings) == 0
		}
		vector, complete := probe()
		if len(vector) >= alertProbeLimit {
			// Broad rules can exceed the result cap. Use one identifying
			// label to narrow the probe without modifying the calculation,
			// then still require a unique compatible output series.
			for _, label := range []string{"instance", "pod", "servicename", "proxy", "mountpoint", "device", "container", "namespace"} {
				if labels[label] != "" {
					form.Set("query", filterAlertOutput(expression, []string{label}, labels))
					vector, complete = probe()
					break
				}
			}
		}
		if complete {
			scope = alertOutputLabels(vector, labels)
		}
	}
	if len(scope) > 0 {
		scoped := scopedAlertExpression(expression, root, scope, labels)
		if validAlertExpression(scoped, dialect) {
			out.Expression = scoped
			status = append([]string{"Scoped by " + strings.Join(scope, ", ")}, status...)
		} else {
			scope = nil
		}
	}
	if len(scope) == 0 {
		status = append(status, "Instance scoping unavailable")
	}
	if out.Expression == original {
		status = append(status, "Original expression used")
	}
	out.Notice = "Alert query · " + strings.Join(status, " · ")
	return out
}

// Only use an output label set that identifies a single compatible series.
// A missing label is not evidence that it can be used as a filter, and a
// disagreement is not resolved by dropping the conflicting label.
func alertOutputLabels(vector model.Vector, labels map[string]string) []string {
	if len(vector) >= alertProbeLimit {
		return nil
	}
	observed := make(map[string]struct{})
	for _, sample := range vector {
		if sample == nil {
			return nil
		}
		for key, value := range labels {
			// Empty values also match a missing label in both dialects.
			if _, ok := sample.Metric[model.LabelName(key)]; ok && value != "" && model.LabelName(key).IsValidLegacy() {
				observed[key] = struct{}{}
			}
		}
	}
	if len(observed) == 0 {
		return nil
	}
	var match *model.Sample
	for _, sample := range vector {
		matches := true
		for key := range observed {
			if string(sample.Metric[model.LabelName(key)]) != labels[key] {
				matches = false
				break
			}
		}
		if matches {
			if match != nil {
				return nil
			}
			match = sample
		}
	}
	if match == nil {
		return nil
	}
	for key, value := range labels {
		if got, ok := match.Metric[model.LabelName(key)]; ok && string(got) != value {
			return nil
		}
	}
	scope := make([]string, 0, len(observed))
	for key := range observed {
		scope = append(scope, key)
	}
	slices.Sort(scope)
	return scope
}

func scopedAlertExpression(expression string, root metricsql.Expr, scope []string, labels map[string]string) string {
	if metric, ok := root.(*metricsql.MetricExpr); ok && len(metric.LabelFilterss) == 1 {
		filters := slices.Clone(metric.LabelFilterss[0])
		for _, key := range scope {
			filter := metricsql.LabelFilter{Label: key, Value: labels[key]}
			if !slices.Contains(filters, filter) {
				filters = append(filters, filter)
			}
		}
		return string((&metricsql.MetricExpr{LabelFilterss: [][]metricsql.LabelFilter{filters}}).AppendString(nil))
	}
	// A synthetic one-series vector performs exact, literal matching at the
	// output, without changing input selection, vector matching or function
	// semantics. This is supported by both Prometheus and VictoriaMetrics.
	return filterAlertOutput(expression, scope, labels)
}

func filterAlertOutput(expression string, scope []string, labels map[string]string) string {
	mask := "vector(1)"
	for _, key := range scope {
		value := strings.ReplaceAll(labels[key], "$", "$$")
		mask = `label_replace(` + mask + `, ` + strconv.Quote(key) + `, ` + strconv.Quote(value) + `, "__name__", "^$")`
	}
	return "(" + expression + "\n) and on (" + strings.Join(scope, ", ") + ") " + mask
}

type alertDialect uint8

var promQLParser = parser.NewParser(parser.Options{})

const (
	prometheusAlert alertDialect = iota
	metricsQLAlert
)

func validAlertExpression(expression string, dialect alertDialect) bool {
	if dialect == prometheusAlert {
		e, err := promQLParser.ParseExpr(expression)
		return err == nil && e.Type() == parser.ValueTypeVector
	}
	_, err := metricsql.Parse(expression)
	return err == nil
}

func alertExpression(original string, allowMetricsQL bool) (expression string, removed bool, dialect alertDialect, root metricsql.Expr) {
	expression = original
	root, metricsErr := metricsql.Parse(original)
	if prom, err := promQLParser.ParseExpr(original); err == nil {
		if prom.Type() != parser.ValueTypeVector {
			return original, false, prometheusAlert, nil
		}
		// Preserve the exact PromQL subtree, including syntax that
		// MetricsQL might normalize differently (e.g. duration literals).
		node := unwrapPromAlert(prom)
		if b, ok := node.(*parser.BinaryExpr); ok && comparison(b.Op.String()) && b.VectorMatching == nil {
			var inner parser.Expr
			switch {
			case promNumber(b.LHS) && b.RHS.Type() == parser.ValueTypeVector:
				inner = b.RHS
			case promNumber(b.RHS) && b.LHS.Type() == parser.ValueTypeVector:
				inner = b.LHS
			}
			if inner != nil {
				pos := inner.PositionRange()
				expression = original[pos.Start:pos.End]
				// Some Prometheus AST ranges end at an inner delimiter
				// (e.g. a vector-matching clause inside a function).
				// Fall back to the AST printer if the span is incomplete.
				if parsed, err := promQLParser.ParseExpr(expression); err != nil || parsed.String() != inner.String() {
					expression = inner.String()
				}
				root, _ = metricsql.Parse(expression)
				return expression, true, prometheusAlert, root
			}
		}
		return original, false, prometheusAlert, root
	}
	if metricsErr != nil || !allowMetricsQL {
		return original, false, metricsQLAlert, nil
	}
	if b, ok := root.(*metricsql.BinaryOpExpr); ok && comparison(b.Op) &&
		b.GroupModifier.Op == "" && b.JoinModifier.Op == "" && !b.KeepMetricNames && b.FillLeft == nil && b.FillRight == nil {
		_, leftNumber := b.Left.(*metricsql.NumberExpr)
		_, rightNumber := b.Right.(*metricsql.NumberExpr)
		if leftNumber != rightNumber {
			inner := b.Left
			if leftNumber {
				inner = b.Right
			}
			hasMetric := false
			metricsql.VisitAll(inner, func(node metricsql.Expr) {
				_, metric := node.(*metricsql.MetricExpr)
				hasMetric = hasMetric || metric
			})
			if hasMetric {
				return string(inner.AppendString(nil)), true, metricsQLAlert, inner
			}
		}
	}
	return original, false, metricsQLAlert, root
}

func comparison(op string) bool {
	switch op {
	case "==", "!=", ">", "<", ">=", "<=":
		return true
	}
	return false
}

func unwrapPromAlert(e parser.Expr) parser.Expr {
	for {
		p, ok := e.(*parser.ParenExpr)
		if !ok {
			return e
		}
		e = p.Expr
	}
}

func promNumber(e parser.Expr) bool {
	switch v := unwrapPromAlert(e).(type) {
	case *parser.NumberLiteral:
		return true
	case *parser.UnaryExpr:
		_, ok := unwrapPromAlert(v.Expr).(*parser.NumberLiteral)
		return ok
	}
	return false
}
