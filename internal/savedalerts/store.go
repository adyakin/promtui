package savedalerts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"

	"gopkg.in/yaml.v3"
	"promtui/internal/saveditems"
)

// Snapshot is an ordered collection and the on-disk version it was read from.
type Snapshot struct {
	path    string
	version []byte
	filters []Filter
}

var ErrChanged = saveditems.ErrChanged

func Open(path string) (Snapshot, error) {
	s := Snapshot{path: path}
	if path == "" {
		return s, errors.New("saved alerts path is not configured")
	}
	raw, err := saveditems.Read(path)
	if err != nil {
		return s, fmt.Errorf("read saved alerts: %w", err)
	}
	if raw == nil {
		return s, nil
	}
	var data collection
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&data); err != nil {
		return s, fmt.Errorf("invalid alerts.yaml: %w", err)
	}
	if err := dec.Decode(new(yaml.Node)); err != io.EOF {
		if err == nil {
			err = errors.New("expected only one YAML document")
		}
		return s, fmt.Errorf("invalid alerts.yaml: %w", err)
	}
	if data.Alerts == nil {
		return s, errors.New("invalid alerts.yaml: expected an alerts list")
	}
	for i, filter := range data.Alerts {
		name, err := checkedName(data.Alerts[:i], filter.Name, -1)
		if err == nil {
			err = filter.validateStored()
		}
		if err != nil {
			return s, fmt.Errorf("invalid alerts.yaml entry %d (%q): %w", i+1, filter.Name, err)
		}
		data.Alerts[i].Name = name
	}
	s.version, s.filters = raw, data.Alerts
	return s, nil
}

func (s Snapshot) Filters() []Filter {
	filters := slices.Clone(s.filters)
	for i := range filters {
		filters[i] = filters[i].clone()
	}
	return filters
}

func checkedName(filters []Filter, name string, except int) (string, error) {
	return saveditems.CheckName(name, len(filters), except, func(i int) string { return filters[i].Name }, "saved alert filter")
}

func (s Snapshot) Add(filter Filter) (Snapshot, error) {
	name, err := checkedName(s.filters, filter.Name, -1)
	if err != nil {
		return s, err
	}
	if err := filter.validateStored(); err != nil {
		return s, err
	}
	filter.Name = name
	return s.commit(append(slices.Clone(s.filters), filter.clone()))
}

func (s Snapshot) Update(index int, filter Filter) (Snapshot, error) {
	if err := s.checkIndex(index); err != nil {
		return s, err
	}
	name, err := checkedName(s.filters, filter.Name, index)
	if err != nil {
		return s, err
	}
	if err := filter.validateStored(); err != nil {
		return s, err
	}
	filter.Name = name
	next := slices.Clone(s.filters)
	next[index] = filter.clone()
	return s.commit(next)
}

func (s Snapshot) Rename(index int, name string) (Snapshot, error) {
	if err := s.checkIndex(index); err != nil {
		return s, err
	}
	name, err := checkedName(s.filters, name, index)
	if err != nil {
		return s, err
	}
	next := slices.Clone(s.filters)
	next[index].Name = name
	return s.commit(next)
}

func (s Snapshot) Delete(index int) (Snapshot, error) {
	if err := s.checkIndex(index); err != nil {
		return s, err
	}
	return s.commit(slices.Delete(slices.Clone(s.filters), index, index+1))
}

func (s Snapshot) checkIndex(index int) error {
	if index < 0 || index >= len(s.filters) {
		return errors.New("select a saved alert filter")
	}
	return nil
}

func (s Snapshot) commit(next []Filter) (Snapshot, error) {
	fail := func(err error) (Snapshot, error) {
		return s, fmt.Errorf("write saved alerts %s: %w", s.path, err)
	}
	if s.path == "" {
		return fail(errors.New("saved alerts path is not configured"))
	}
	raw, err := yaml.Marshal(collection{next})
	if err != nil {
		return fail(err)
	}
	if err := saveditems.Write(s.path, s.version, raw); err != nil {
		return fail(err)
	}
	s.version, s.filters = raw, next
	return s, nil
}
