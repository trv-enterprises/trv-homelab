// Package rules reads Marshal's rules.yaml (bind-mounted read-only) so
// sentinel can report the rule list and know each rule's repeat window for
// the staleness sweep. It never writes the file: rules change through the
// deploy path, not through this server.
package rules

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	Name          string `json:"name"`
	Topic         string `json:"topic"`
	Severity      string `json:"severity"`
	RepeatMinutes int    `json:"repeat_minutes"`
	// HasAlert is false for a rule whose alert block is switched off
	// (alert.active: false, Marshal 0.5.0): Marshal never emits for it, so
	// it is not a notification source.
	HasAlert    bool   `json:"has_alert"`
	HasAction   bool   `json:"has_action"`
	EnableTopic string `json:"enable_topic,omitempty"`
	StateTopic  string `json:"state_topic,omitempty"`
	// ActiveTopic is the action's runtime switch (Marshal 0.5.0): a retained
	// true/false there overrides ActiveDefault, the configured action.active.
	ActiveTopic   string `json:"active_topic,omitempty"`
	ActiveDefault bool   `json:"-"`
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
			Active        *bool  `yaml:"active"`
		} `yaml:"alert"`
		Action *struct {
			EnableTopic string `yaml:"enable_topic"`
			StateTopic  string `yaml:"state_topic"`
			Active      *bool  `yaml:"active"`
			ActiveTopic string `yaml:"active_topic"`
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
			if r.Alert.Active != nil && !*r.Alert.Active {
				rule.HasAlert = false
			}
		}
		if rule.HasAlert && rule.Severity == "" {
			rule.Severity = "warning" // Marshal's default
		}
		if r.Action != nil {
			rule.HasAction = true
			rule.EnableTopic, rule.StateTopic = r.Action.EnableTopic, r.Action.StateTopic
			rule.ActiveTopic = r.Action.ActiveTopic
			rule.ActiveDefault = r.Action.Active == nil || *r.Action.Active
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

// RetainedTopics returns the distinct state_topic and active_topic values
// across all rules: the Marshal topics whose retained value sentinel mirrors.
func (r *Reader) RetainedTopics() []string {
	rules, _ := r.Rules()
	seen := map[string]bool{}
	var out []string
	for _, rule := range rules {
		for _, t := range []string{rule.StateTopic, rule.ActiveTopic} {
			if t != "" && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// ParseActive reads an active_topic payload the way Marshal does: a bare
// word (true/false, on/off, 1/0, enable(d)/disable(d), active/inactive) or a
// JSON object {"active": <bool|word>}. ok is false for anything else, the
// empty payload of a cleared retained value included, and the caller falls
// back to the rule's configured default.
func ParseActive(payload string) (active, ok bool) {
	word := strings.TrimSpace(payload)
	if strings.HasPrefix(word, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(word), &obj); err != nil {
			return false, false
		}
		switch v := obj["active"].(type) {
		case bool:
			return v, true
		case string:
			word = v
		default:
			return false, false
		}
	}
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "true", "on", "1", "enable", "enabled", "active":
		return true, true
	case "false", "off", "0", "disable", "disabled", "inactive":
		return false, true
	}
	return false, false
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
