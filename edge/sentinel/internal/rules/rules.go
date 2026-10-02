// Package rules reads Marshal's rules.yaml (bind-mounted read-only) so
// sentinel can report the rule list and know each rule's repeat window for
// the staleness sweep. It never writes the file: rules change through the
// deploy path, not through this server.
package rules

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	Name          string `json:"name"`
	Topic         string `json:"topic"`
	Severity      string `json:"severity"`
	RepeatMinutes int    `json:"repeat_minutes"`
	HasAlert      bool   `json:"has_alert"`
	HasAction     bool   `json:"has_action"`
	EnableTopic   string `json:"enable_topic,omitempty"`
	StateTopic    string `json:"state_topic,omitempty"`
}

// file mirrors just the parts of Marshal's schema we read. Flat legacy
// fields (duration_minutes/repeat_minutes/severity/message at rule level)
// are honored the same way Marshal folds them into alert:.
type file struct {
	Rules []struct {
		Name          string `yaml:"name"`
		Topic         string `yaml:"topic"`
		Severity      string `yaml:"severity"`
		RepeatMinutes int    `yaml:"repeat_minutes"`
		Message       string `yaml:"message"`
		Alert         *struct {
			Severity      string `yaml:"severity"`
			RepeatMinutes int    `yaml:"repeat_minutes"`
		} `yaml:"alert"`
		Action *struct {
			EnableTopic string `yaml:"enable_topic"`
			StateTopic  string `yaml:"state_topic"`
		} `yaml:"action"`
	} `yaml:"rules"`
}

// Parse reads rules from YAML bytes.
func Parse(b []byte) ([]Rule, error) {
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(f.Rules))
	for _, r := range f.Rules {
		rule := Rule{Name: r.Name, Topic: r.Topic, Severity: r.Severity, RepeatMinutes: r.RepeatMinutes}
		rule.HasAlert = r.Alert != nil || r.Message != ""
		if r.Alert != nil {
			rule.Severity, rule.RepeatMinutes = r.Alert.Severity, r.Alert.RepeatMinutes
		}
		if rule.HasAlert && rule.Severity == "" {
			rule.Severity = "warning" // Marshal's default
		}
		if r.Action != nil {
			rule.HasAction = true
			rule.EnableTopic, rule.StateTopic = r.Action.EnableTopic, r.Action.StateTopic
		}
		out = append(out, rule)
	}
	return out, nil
}

// Reader re-reads the file lazily, at most once per ttl. A missing or
// unparseable file keeps the last good result (or an empty list) so a
// deploy that briefly removes the file never breaks the API.
type Reader struct {
	path string
	ttl  time.Duration

	mu     sync.Mutex
	rules  []Rule
	loaded time.Time
	err    error
}

func NewReader(path string, ttl time.Duration) *Reader {
	return &Reader{path: path, ttl: ttl}
}

// Rules returns the current rule list. err is the last read error, if any,
// alongside the last good list.
func (r *Reader) Rules() ([]Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.loaded) < r.ttl && r.rules != nil {
		return r.rules, r.err
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		r.err = err
		r.loaded = time.Now()
		if r.rules == nil {
			r.rules = []Rule{}
		}
		return r.rules, err
	}
	rules, err := Parse(b)
	r.err = err
	r.loaded = time.Now()
	if err == nil {
		r.rules = rules
	} else if r.rules == nil {
		r.rules = []Rule{}
	}
	return r.rules, r.err
}

// RepeatWindow returns the repeat interval for a rule, or 0 if unknown.
func (r *Reader) RepeatWindow(name string) time.Duration {
	rules, _ := r.Rules()
	for _, rule := range rules {
		if rule.Name == name {
			return time.Duration(rule.RepeatMinutes) * time.Minute
		}
	}
	return 0
}

// StateTopics returns the distinct state_topic values across all rules.
func (r *Reader) StateTopics() []string {
	rules, _ := r.Rules()
	seen := map[string]bool{}
	var out []string
	for _, rule := range rules {
		if rule.StateTopic != "" && !seen[rule.StateTopic] {
			seen[rule.StateTopic] = true
			out = append(out, rule.StateTopic)
		}
	}
	return out
}

// Find returns the rule with the given name.
func (r *Reader) Find(name string) (Rule, bool) {
	rules, _ := r.Rules()
	for _, rule := range rules {
		if rule.Name == name {
			return rule, true
		}
	}
	return Rule{}, false
}
