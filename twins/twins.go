// Package twins implements a framework for testing HotStuff implementations.
package twins

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// ScenarioSource is a source of twins scenarios to execute.
type ScenarioSource interface {
	Settings() Settings
	NextScenario() (Scenario, error)
	Remaining() int64
}

type twinsJSON struct {
	NumNodes   uint8             `json:"num_nodes"`
	NumTwins   uint8             `json:"num_twins"`
	Partitions uint8             `json:"partitions"`
	Views      uint8             `json:"views"`
	Ticks      int               `json:"ticks"`
	Shuffle    bool              `json:"shuffle"`
	Seed       int64             `json:"seed"`
	Scenarios  []json.RawMessage `json:"scenarios"`

	scenario int
}

func (t twinsJSON) Settings() Settings {
	return Settings{
		NumNodes:   t.NumNodes,
		NumTwins:   t.NumTwins,
		Partitions: t.Partitions,
		Views:      t.Views,
		Ticks:      t.Ticks,
		Shuffle:    t.Shuffle,
		Seed:       t.Seed,
	}
}

func (t *twinsJSON) NextScenario() (Scenario, error) {
	var s Scenario
	err := json.Unmarshal(t.Scenarios[t.scenario], &s)
	t.scenario++
	return s, err
}

func (t *twinsJSON) Remaining() int64 {
	return int64(len(t.Scenarios) - t.scenario)
}

// FromJSON returns a scenario source that reads from the given reader.
func FromJSON(rd io.Reader) (ScenarioSource, error) {
	var root twinsJSON
	dec := json.NewDecoder(rd)
	err := dec.Decode(&root)
	if err != nil {
		return nil, err
	}

	return &root, nil
}

// Settings contains the settings used with the scenario generator.
type Settings struct {
	NumNodes   uint8
	NumTwins   uint8
	Partitions uint8
	Views      uint8
	Ticks      int
	Shuffle    bool
	Seed       int64
}

// JSONWriter writes scenarios to JSON.
type JSONWriter struct {
	mut   sync.Mutex
	wr    io.Writer
	first bool
}

// WriteScenario writes a single scenario to the JSON stream.
func (jwr *JSONWriter) WriteScenario(s Scenario) error {
	buf, err := json.Marshal(s)
	if err != nil {
		return err
	}
	jwr.mut.Lock()
	defer jwr.mut.Unlock()
	if jwr.first {
		_, err = io.WriteString(jwr.wr, "\n\t\t")
		jwr.first = false
	} else {
		_, err = io.WriteString(jwr.wr, ",\n\t\t")
	}
	if err != nil {
		return err
	}
	_, err = jwr.wr.Write(buf)
	return err
}

// Close closes the JSON stream.
func (jwr *JSONWriter) Close() error {
	tail := "\n\t]\n}"
	_, err := io.WriteString(jwr.wr, tail)
	return err
}

// ToJSON returns a JSONWriter that can be used to write scenarios as JSON.
func ToJSON(settings Settings, wr io.Writer) (*JSONWriter, error) {
	head := fmt.Sprintf(`{
	"num_nodes": %d,
	"num_twins": %d,
	"partitions": %d,
	"views": %d,
	"ticks": %d,
	"shuffle": %t,
	"seed": %d,
	"scenarios": [`,
		settings.NumNodes,
		settings.NumTwins,
		settings.Partitions,
		settings.Views,
		settings.Ticks,
		settings.Shuffle,
		settings.Seed,
	)

	_, err := io.WriteString(wr, head)
	if err != nil {
		return nil, err
	}
	return &JSONWriter{wr: wr, first: true}, nil
}
