package alertmanager

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/matcher/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

// Routing is a read-only snapshot. No raw YAML or integration credentials are
// retained. Order contains route IDs in depth-first configuration order.
type Routing struct {
	Routes     map[string]*Route
	Order      []string
	Receivers  []Receiver
	References map[string][]string // effective receiver name → route IDs
}

const RootRoute = "0"

type Route struct {
	ID, Parent                               string
	Children                                 []string
	Depth                                    int
	Matchers                                 []*labels.Matcher
	Receiver                                 Setting[string]
	GroupBy                                  Setting[[]string]
	GroupWait, GroupInterval, RepeatInterval Setting[model.Duration]
	Continue                                 bool
}

// Source is the ID of the route that set this value (empty for a default).
type Setting[T any] struct {
	Value  T
	Source string
}

type Receiver struct {
	Name         string
	Integrations []Integration
}

type Integration struct {
	Type, DestinationLabel, Destination string
}

func (c Client) Routing(ctx context.Context) (*Routing, error) {
	original, err := c.originalConfig(ctx)
	if err != nil {
		return nil, err
	}
	return parseRouting(original)
}

func (c Client) originalConfig(ctx context.Context) (string, error) {
	var status struct {
		Config struct{ Original string }
	}
	if err := c.request(ctx, http.MethodGet, "api/v2/status", "", nil, &status); err != nil {
		return "", err
	}
	if strings.TrimSpace(status.Config.Original) == "" {
		return "", errors.New("invalid Alertmanager status response: missing config.original")
	}
	return status.Config.Original, nil
}

// Project just the fields used by the explorer rather than validating notifier
// settings against a single Alertmanager version. Runtime YAML may contain
// redacted fields and integrations newer than our matcher library.
type routeYAML struct {
	Receiver       string
	Routes         []*routeYAML
	Matchers       []string
	Match          map[string]string
	MatchRE        map[string]string `yaml:"match_re"`
	GroupBy        []string          `yaml:"group_by"`
	GroupWait      *model.Duration   `yaml:"group_wait"`
	GroupInterval  *model.Duration   `yaml:"group_interval"`
	RepeatInterval *model.Duration   `yaml:"repeat_interval"`
	Continue       bool
}

func parseRouting(original string) (*Routing, error) {
	var config struct {
		Route     *routeYAML
		Receivers []yaml.Node
	}
	if err := yaml.Unmarshal([]byte(original), &config); err != nil {
		// YAML and matcher errors may quote input containing credentials.
		return nil, errors.New("could not parse Alertmanager configuration YAML")
	}
	if config.Route == nil {
		return nil, errors.New("invalid Alertmanager configuration: missing root route")
	}
	r := &Routing{Routes: map[string]*Route{}, References: map[string][]string{}}
	defaults := Route{
		GroupWait:      Setting[model.Duration]{Value: model.Duration(30 * time.Second)},
		GroupInterval:  Setting[model.Duration]{Value: model.Duration(5 * time.Minute)},
		RepeatInterval: Setting[model.Duration]{Value: model.Duration(4 * time.Hour)},
	}
	if !r.addRoute(config.Route, &defaults, RootRoute, 0) {
		return nil, errors.New("invalid Alertmanager configuration: invalid route or matcher")
	}
	seen := map[string]bool{}
	for i := range config.Receivers {
		receiver, ok := readReceiver(&config.Receivers[i])
		if !ok || seen[receiver.Name] {
			return nil, errors.New("invalid Alertmanager configuration: invalid or duplicate receiver")
		}
		seen[receiver.Name] = true
		r.Receivers = append(r.Receivers, receiver)
	}
	return r, nil
}

func (r *Routing) addRoute(raw *routeYAML, parent *Route, id string, depth int) bool {
	if raw == nil {
		return false
	}
	node := &Route{
		ID: id, Parent: parent.ID, Depth: depth, Continue: raw.Continue,
		Receiver: parent.Receiver, GroupBy: parent.GroupBy, GroupWait: parent.GroupWait,
		GroupInterval: parent.GroupInterval, RepeatInterval: parent.RepeatInterval,
	}
	if raw.Receiver != "" {
		node.Receiver = Setting[string]{raw.Receiver, id}
	}
	// Alertmanager builds its internal GroupBy by appending one entry per
	// label, so both an empty list and omission inherit the parent's labels.
	if len(raw.GroupBy) != 0 {
		node.GroupBy = Setting[[]string]{raw.GroupBy, id}
	}
	for _, pair := range []struct {
		value *model.Duration
		dest  *Setting[model.Duration]
	}{{raw.GroupWait, &node.GroupWait}, {raw.GroupInterval, &node.GroupInterval}, {raw.RepeatInterval, &node.RepeatInterval}} {
		if pair.value != nil {
			*pair.dest = Setting[model.Duration]{*pair.value, id}
		}
	}
	matchers, err := configMatchers(raw.Matchers, raw.Match, raw.MatchRE)
	if err != nil {
		return false
	}
	node.Matchers = matchers
	r.Routes[id] = node
	r.Order = append(r.Order, id)
	if node.Receiver.Value != "" {
		r.References[node.Receiver.Value] = append(r.References[node.Receiver.Value], id)
	}
	for i, child := range raw.Routes {
		childID := id + "/" + strconv.Itoa(i)
		node.Children = append(node.Children, childID)
		if !r.addRoute(child, node, childID, depth+1) {
			return false
		}
	}
	return true
}

func configMatchers(current []string, exact, regex map[string]string) (labels.Matchers, error) {
	var matchers labels.Matchers
	for _, legacy := range []struct {
		values map[string]string
		op     labels.MatchType
	}{{exact, labels.MatchEqual}, {regex, labels.MatchRegexp}} {
		for name, value := range legacy.values {
			m, err := labels.NewMatcher(legacy.op, name, value)
			if err != nil {
				return nil, err
			}
			matchers = append(matchers, m)
		}
	}
	for _, s := range current {
		// Mirror Alertmanager's compatibility mode without logging input.
		// Prefer the classic interpretation in case of disagreement.
		m, err := labels.ParseMatcher(s)
		if err != nil {
			m, err = parse.Matcher(s)
			if err != nil {
				return nil, err
			}
		}
		matchers = append(matchers, m)
	}
	slices.SortFunc(matchers, func(a, b *labels.Matcher) int {
		return strings.Compare(a.String(), b.String())
	})
	return matchers, nil
}

func yamlValue(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}

var integrationKey = regexp.MustCompile(`^[a-z][a-z0-9_]*_configs$`)

func readReceiver(n *yaml.Node) (Receiver, bool) {
	var fields map[string]yaml.Node
	if yamlValue(n).Decode(&fields) != nil {
		return Receiver{}, false
	}
	name := fields["name"]
	var receiver Receiver
	if name.Decode(&receiver.Name) != nil || receiver.Name == "" {
		return Receiver{}, false
	}
	var keys []string
	for key := range fields {
		if integrationKey.MatchString(key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		configs := fields[key]
		n := yamlValue(&configs)
		if n.Kind != yaml.SequenceNode {
			continue
		}
		kind := strings.TrimSuffix(key, "_configs")
		for _, entry := range n.Content {
			receiver.Integrations = append(receiver.Integrations, readIntegration(kind, yamlValue(entry)))
		}
	}
	return receiver, true
}

func readIntegration(kind string, entry *yaml.Node) Integration {
	i := Integration{Type: kind}
	var key string
	switch kind {
	case "telegram":
		i.Type, i.DestinationLabel, key = "Telegram", "Chat ID", "chat_id"
	case "slack":
		i.Type, i.DestinationLabel, key = "Slack", "Channel", "channel"
	case "email":
		i.Type, i.DestinationLabel, key = "Email", "To", "to"
	case "webhook":
		i.Type, i.DestinationLabel, key = "Webhook", "Host", "url"
	case "msteams", "msteamsv2":
		i.Type, i.DestinationLabel, key = "Microsoft Teams", "Host", "webhook_url"
	case "discord":
		i.Type, i.DestinationLabel, key = "Discord", "Host", "webhook_url"
	case "pagerduty":
		i.Type = "PagerDuty"
	case "opsgenie":
		i.Type = "Opsgenie"
	case "pushover":
		i.Type = "Pushover"
	default:
		return i
	}
	if key == "" {
		return i
	}
	var fields map[string]yaml.Node
	if entry.Decode(&fields) != nil {
		return i
	}
	var value string
	n := fields[key]
	if n.Decode(&value) != nil {
		return i
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "<secret>") || strings.EqualFold(value, "<redacted>") ||
		strings.Contains(value, "{{") {
		return i
	}
	if i.DestinationLabel == "Host" {
		// Never expose userinfo, path, query or fragment, even if the URL
		// isn't recognized as a particular webhook provider.
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
			return i
		}
		value = u.Host
	}
	i.Destination = value
	return i
}
