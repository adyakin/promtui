package alertmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSilencesV2(t *testing.T) {
	const list, detail = "/proxy/api/v2/silences", "/proxy/api/v2/silence/old"
	const record = `{"id":"old","startsAt":"2026-01-01T02:00:00Z","endsAt":"2026-01-01T03:00:00Z",
		"updatedAt":"2026-01-01T02:30:00Z","createdBy":"ops","comment":"test",
		"status":{"state":"active"},"matchers":[{"name":"alertname","value":"test","isRegex":false}]}`
	specs := make(chan SilenceSpec, 2)
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET " + list:
			fmt.Fprint(w, "["+record+"]")
		case "GET " + detail:
			fmt.Fprint(w, record)
		case "POST " + list:
			var spec SilenceSpec
			if err := json.NewDecoder(r.Body).Decode(&spec); r.Header.Get("Content-Type") != "application/json" || err != nil {
				t.Errorf("invalid POST: %s, %v", r.Header.Get("Content-Type"), err)
			}
			specs <- spec
			fmt.Fprint(w, `{"silenceID":"returned"}`)
		case "DELETE " + detail:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	c := Client{Base: s.URL + "/proxy/", HTTP: s.Client()}
	ctx := t.Context()
	got, err := c.Silences(ctx)
	if err != nil || len(got) != 1 || got[0].Matchers[0].Operator() != "=" || got[0].UpdatedAt.Minute() != 30 {
		t.Fatalf("list: %+v, %v", got, err)
	}
	row, err := c.Silence(ctx, "old")
	if err != nil || !reflect.DeepEqual(got[0], row) {
		t.Fatalf("get: %+v, %v", row, err)
	}
	spec := row.SilenceSpec
	spec.Matchers = []Matcher{{Name: "job", Value: " unusual\t value ", IsEqual: false, IsRegex: true}}
	for _, want := range []string{"", row.ID} {
		spec.ID = want
		id, err := c.SaveSilence(ctx, spec)
		if err != nil || id != "returned" {
			t.Fatalf("save %q: %q %v", want, id, err)
		}
		select {
		case sent := <-specs:
			if sent.ID != want || !slices.Equal(sent.Matchers, spec.Matchers) || !sent.StartsAt.Equal(spec.StartsAt) ||
				!sent.EndsAt.Equal(spec.EndsAt) || sent.CreatedBy != spec.CreatedBy || sent.Comment != spec.Comment {
				t.Fatalf("wrong POST payload: %+v", sent)
			}
		default:
			t.Fatal("POST did not reach Alertmanager")
		}
	}
	if err := c.ExpireSilence(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSilenceAPIErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		match  string
	}{
		{http.StatusBadRequest, `"invalid matcher"`, "invalid matcher"},
		{http.StatusForbidden, "secret", "authorization"},
		{http.StatusConflict, "", "HTTP 409"},
		{http.StatusTemporaryRedirect, "", "HTTP 307"},
		{http.StatusOK, `{}`, "did not return a silence ID"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/silences" {
					t.Errorf("write followed redirect: %s", r.URL)
				}
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			_, err := (Client{Base: s.URL}).SaveSilence(t.Context(), SilenceSpec{})
			if err == nil || !strings.Contains(err.Error(), tc.match) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	c := Client{Base: s.URL}
	if _, err := c.Silence(t.Context(), "missing"); !errors.Is(err, ErrSilenceNotFound) {
		t.Fatal(err)
	}
	if err := c.ExpireSilence(t.Context(), "missing"); !errors.Is(err, ErrSilenceNotFound) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Silences(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCompileSilenceMatchers(t *testing.T) {
	for _, tt := range []struct {
		op             string
		pattern, value string
		missing, want  bool
	}{
		{"=", " a\t ", " a\t ", false, true},
		{"=", "", "", true, true},
		{"!=", "a", "", true, true},
		{"!=", "", "", false, false},
		{"=~", "a|b", "aa", false, false},
		{"=~", "a|b", "b", false, true},
		{"=~", "a.*", "a\nb", false, false},
		{"=~", "(?s)a.*", "a\nb", false, true},
		{"!~", "a|b", "aa", false, true},
		{"!~", ".*", "", true, false},
		{"!~", "a", "a", false, false},
	} {
		m := Matcher{Name: "label", Value: tt.pattern, IsRegex: strings.HasSuffix(tt.op, "~"), IsEqual: !strings.HasPrefix(tt.op, "!")}
		labels := map[string]string{"job": "worker"}
		if !tt.missing {
			labels["label"] = tt.value
		}
		predicate, _, err := CompileMatchers([]Matcher{m, {Name: "job", Value: "worker", IsEqual: true}})
		if err != nil || predicate(labels) != tt.want {
			t.Fatalf("%v: matcher did not behave as expected (%v)", tt, err)
		}
		delete(labels, "job")
		if predicate(labels) {
			t.Fatal("matchers were not ANDed")
		}
	}
	if _, _, err := CompileMatchers([]Matcher{{Name: "key", Value: "[", IsEqual: true, IsRegex: true}}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	now := time.Now()
	_, broad, err := CompileMatchers([]Matcher{{Name: "key", Value: "x", IsEqual: false}})
	if err != nil || !broad {
		t.Fatal("missing-label matcher did not trigger warning")
	}
	if (Silence{SilenceSpec: SilenceSpec{StartsAt: now, EndsAt: now.Add(time.Minute)}}).State(now) != "active" {
		t.Fatal("wrong status on start boundary")
	}
}
