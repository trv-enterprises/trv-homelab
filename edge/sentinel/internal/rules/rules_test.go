package rules

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const sample = `
mqtt: {broker: "tcp://mosquitto:1883"}
alert_topic: sensors/alerts
rules:
  - name: garage
    topic: "zigbee2mqtt/Large Garage Door"
    condition: {field: contact, operator: eq, value: false}
    duration_minutes: 30
    repeat_minutes: 60
    severity: warning
    message: "open {duration}"
  - name: freezer
    topic: "zigbee2mqtt/Freezer"
    condition: {field: temp, operator: gt, value: 0}
    alert: {duration_minutes: 5, repeat_minutes: 15, severity: critical, message: "warm"}
  - name: nightlight_kitchen
    topic: "zigbee2mqtt/night-light-kitchen"
    condition: {field: occupancy, operator: eq, value: true}
    action:
      topic: "zigbee2mqtt/night-light-kitchen/set"
      payload: '{"state":"ON"}'
      enable_topic: automation/night-light/enable
      state_topic: automation/night-light-kitchen/owner
  - name: nightlight_hall
    topic: "zigbee2mqtt/night-light-hall"
    condition: {field: occupancy, operator: eq, value: true}
    alert: {duration_minutes: 0, severity: info, message: "Motion in the hall"}
    action:
      topic: "zigbee2mqtt/night-light-hall/set"
      payload: '{"state":"ON"}'
      enable_topic: automation/night-light/enable
      state_topic: automation/night-light-hall/owner
      active_topic: automation/night-light-hall/active
  - name: switched_off
    topic: "zigbee2mqtt/Office"
    condition: {field: temp, operator: gt, value: 76}
    alert: {severity: warning, message: "warm", active: false}
    action: {topic: "notify/fan/set", payload: '{"state":"ON"}', active: false}
`

func TestParse(t *testing.T) {
	rules, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 5 {
		t.Fatalf("got %d rules", len(rules))
	}
	g := rules[0]
	if !g.HasAlert || g.HasAction || g.RepeatMinutes != 60 || g.Severity != "warning" {
		t.Fatalf("garage %+v", g)
	}
	f := rules[1]
	if !f.HasAlert || f.RepeatMinutes != 15 || f.Severity != "critical" {
		t.Fatalf("freezer %+v", f)
	}
	n := rules[2]
	if n.HasAlert || !n.HasAction || n.EnableTopic != "automation/night-light/enable" || n.StateTopic == "" {
		t.Fatalf("nightlight %+v", n)
	}
	// No active_topic and no action.active: the action is active.
	if n.ActiveTopic != "" || !n.ActiveDefault {
		t.Fatalf("nightlight active %+v", n)
	}
	h := rules[3]
	if !h.HasAlert || !h.HasAction || h.Severity != "info" || h.ActiveTopic != "automation/night-light-hall/active" || !h.ActiveDefault {
		t.Fatalf("hall %+v", h)
	}
	// alert.active: false is not a source; action.active: false is the default.
	o := rules[4]
	if o.HasAlert || !o.HasAction || o.ActiveDefault {
		t.Fatalf("switched off %+v", o)
	}
}

func TestRetainedTopics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewReader(path, time.Minute).RetainedTopics()
	want := []string{
		"automation/night-light-kitchen/owner",
		"automation/night-light-hall/owner",
		"automation/night-light-hall/active",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// The accepted words are Marshal's (engine.parseActive): if these drift, the
// app shows a switch position Marshal is not in.
func TestParseActive(t *testing.T) {
	on := []string{"true", "on", "1", "enable", "enabled", "active", " TRUE\n", "On", `{"active": true}`, `{"active": "on"}`, `{"active": "active"}`}
	off := []string{"false", "off", "0", "disable", "disabled", "inactive", "Inactive", `{"active": false}`, `{"active": "off"}`}
	bad := []string{"", "  ", "yes", "{}", `{"enable": false}`, `{"active": 0}`, `{"active": null}`, `{"active"`, "automation"}
	for _, p := range on {
		if v, ok := ParseActive(p); !ok || !v {
			t.Errorf("%q: got %v, %v want active", p, v, ok)
		}
	}
	for _, p := range off {
		if v, ok := ParseActive(p); !ok || v {
			t.Errorf("%q: got %v, %v want inactive", p, v, ok)
		}
	}
	for _, p := range bad {
		if _, ok := ParseActive(p); ok {
			t.Errorf("%q: should not parse", p)
		}
	}
}
