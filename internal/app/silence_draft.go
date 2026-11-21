package app

import (
	"errors"
	"fmt"
	"maps"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"promtui/internal/alertmanager"
)

const localSilenceTime = "2006-01-02 15:04"
const localSilencePlaceholder = "YYYY-MM-DD HH:MM"

var durationPart = regexp.MustCompile(`^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(ns|µs|μs|us|ms|s|m|h|d)`)

func parseSilenceDuration(value string) (time.Duration, error) {
	s := strings.TrimSpace(value)
	if !strings.Contains(s, "d") {
		return time.ParseDuration(s)
	}
	var out strings.Builder
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		out.WriteByte(s[0])
		s = s[1:]
	}
	for len(s) > 0 {
		p := durationPart.FindStringSubmatch(s)
		if p == nil {
			return 0, errors.New("invalid duration")
		}
		if p[2] == "d" {
			d, ok := new(big.Rat).SetString(p[1])
			if !ok {
				return 0, errors.New("invalid day duration")
			}
			d.Mul(d, big.NewRat(int64(24*time.Hour), 1))
			out.WriteString(new(big.Int).Quo(d.Num(), d.Denom()).String() + "ns")
		} else {
			out.WriteString(p[0])
		}
		s = s[len(p[0]):]
	}
	return time.ParseDuration(out.String())
}

func parseSilenceTime(s string, loc *time.Location) (time.Time, error) {
	wall, err := time.Parse(localSilenceTime, s)
	if err != nil || wall.Format(localSilenceTime) != s {
		return time.Time{}, errors.New("use " + localSilencePlaceholder)
	}
	// Probe all zone intervals around the wall clock date, not merely the
	// offset chosen by ParseInLocation (which cannot detect a repeated time).
	var result time.Time
	var count int
	offsets := make(map[int]bool)
	for probe, end := wall.Add(-48*time.Hour), wall.Add(48*time.Hour); probe.Before(end); {
		local := probe.In(loc)
		_, offset := local.Zone()
		if !offsets[offset] {
			offsets[offset] = true
			candidate := wall.Add(-time.Duration(offset) * time.Second).In(loc)
			if candidate.Format(localSilenceTime) == s {
				count++
				result = candidate
			}
		}
		_, next := local.ZoneBounds()
		if next.IsZero() || !next.After(probe) {
			break
		}
		probe = next
	}
	switch count {
	case 0:
		return time.Time{}, errors.New("local time does not exist; choose a time outside the clock change")
	case 1:
		return result, nil
	default:
		return time.Time{}, errors.New("local time is ambiguous; choose a time outside the clock change")
	}
}

type silenceMatcherDraft struct {
	name, value textinput.Model
	operator    int
	quoted      bool
}

var silenceOperators = [...]string{"=", "!=", "=~", "!~"}

func silenceInput(value, placeholder string) textinput.Model {
	t := textinput.New()
	t.Prompt, t.Placeholder = "", placeholder
	t.SetVirtualCursor(false)
	t.SetValue(value)
	return t
}

func newSilenceMatcher(m alertmanager.Matcher) silenceMatcherDraft {
	op := max(0, slices.Index(silenceOperators[:], m.Operator()))
	quoted := strings.ContainsFunc(m.Value, unicode.IsControl)
	value, hint := m.Value, "value (empty allowed)"
	if quoted {
		value, hint = strconv.Quote(value), `quoted value, e.g. "a\nb"`
	}
	input := silenceInput(value, hint)
	if quoted {
		input.SetCursor(input.Position() - 1)
	}
	return silenceMatcherDraft{silenceInput(m.Name, "label name"), input, op, quoted}
}

type silenceDraft struct {
	original          *alertmanager.Silence
	fromAlert, copied bool
	scheduled, until  bool
	start, end        textinput.Model
	duration          textinput.Model
	creator, comment  textinput.Model
	originalComment   string
	commentDisplay    string
	matchers          []silenceMatcherDraft
	focus             int
	err               error
	preview           silenceMatches
}

func newSilenceDraft(creator string, source *alertmanager.Silence, labels map[string]string, fromAlert bool, now time.Time) *silenceDraft {
	d := &silenceDraft{fromAlert: fromAlert, start: silenceInput("", localSilencePlaceholder), end: silenceInput("", localSilencePlaceholder),
		duration: silenceInput("2h", "e.g. 30m, 2h, 7d"), creator: silenceInput(creator, "required"), comment: silenceInput("", "required")}
	if source != nil {
		d.creator.SetValue(source.CreatedBy)
		d.comment.SetValue(source.Comment)
		d.originalComment, d.commentDisplay = source.Comment, d.comment.Value()
		for _, matcher := range source.Matchers {
			d.matchers = append(d.matchers, newSilenceMatcher(matcher))
		}
		if source.State(now) != "expired" {
			d.original = source
			d.scheduled, d.until = true, true
			d.start.SetValue(source.StartsAt.Local().Format(localSilenceTime))
			d.end.SetValue(source.EndsAt.Local().Format(localSilenceTime))
		} else {
			d.copied = true
		}
	} else {
		for _, k := range slices.Sorted(maps.Keys(labels)) {
			d.matchers = append(d.matchers, newSilenceMatcher(alertmanager.Matcher{Name: k, Value: labels[k], IsEqual: true}))
		}
	}
	if len(d.matchers) == 0 {
		d.matchers = append(d.matchers, newSilenceMatcher(alertmanager.Matcher{IsEqual: true}))
	}
	return d
}

func (d *silenceDraft) matcherValues() ([]alertmanager.Matcher, error) {
	rows := make([]alertmanager.Matcher, len(d.matchers))
	for i, m := range d.matchers {
		v := m.value.Value()
		if m.quoted {
			var err error
			v, err = strconv.Unquote(v)
			if err != nil {
				return nil, fmt.Errorf("matcher %d: enter a valid double-quoted value", i+1)
			}
		}
		rows[i] = alertmanager.Matcher{Name: m.name.Value(), Value: v, IsEqual: m.operator%2 == 0, IsRegex: m.operator >= 2}
	}
	return rows, nil
}

func (d *silenceDraft) spec(now time.Time) (alertmanager.SilenceSpec, bool, error) {
	var spec alertmanager.SilenceSpec
	spec.CreatedBy, spec.Comment = d.creator.Value(), d.comment.Value()
	if d.originalComment != "" && spec.Comment == d.commentDisplay {
		spec.Comment = d.originalComment
	}
	if strings.TrimSpace(spec.CreatedBy) == "" {
		return spec, false, errors.New("enter a creator")
	}
	if strings.TrimSpace(spec.Comment) == "" {
		return spec, false, errors.New("enter a comment")
	}
	if d.original != nil {
		spec.ID, spec.Annotations = d.original.ID, d.original.Annotations
	}
	start, err := d.startTime(now)
	if err != nil {
		return spec, false, fmt.Errorf("start: %w", err)
	}
	end, err := d.endTime(start)
	if err != nil {
		return spec, false, fmt.Errorf("expiration: %w", err)
	}
	if !end.After(start) || !end.After(now) {
		return spec, false, errors.New("expiration must be after both the start and the current time")
	}
	spec.StartsAt, spec.EndsAt = start.UTC(), end.UTC()
	spec.Matchers, err = d.matcherValues()
	if err != nil {
		return spec, false, err
	}
	matches, broad, err := alertmanager.CompileMatchers(spec.Matchers)
	if err == nil && matches(nil) && !slices.ContainsFunc(spec.Matchers, func(m alertmanager.Matcher) bool { return !m.IsEqual }) {
		err = errors.New("at least one matcher must use a non-empty label value")
	}
	return spec, broad, err
}

func (d *silenceDraft) startTime(now time.Time) (time.Time, error) {
	if !d.scheduled {
		return now, nil
	}
	if d.original != nil && d.start.Value() == d.original.StartsAt.Local().Format(localSilenceTime) {
		return d.original.StartsAt, nil
	}
	t, err := parseSilenceTime(d.start.Value(), time.Local)
	if err == nil && !t.After(now) {
		err = errors.New("choose a future start or select Now")
	}
	return t, err
}

func (d *silenceDraft) endTime(start time.Time) (time.Time, error) {
	if d.until {
		if d.original != nil && d.end.Value() == d.original.EndsAt.Local().Format(localSilenceTime) {
			return d.original.EndsAt, nil
		}
		return parseSilenceTime(d.end.Value(), time.Local)
	}
	t, err := parseSilenceDuration(d.duration.Value())
	if err != nil || t <= 0 {
		return time.Time{}, errors.New("enter a positive duration, e.g. 30m, 2h or 7d")
	}
	return start.Add(t), nil
}
