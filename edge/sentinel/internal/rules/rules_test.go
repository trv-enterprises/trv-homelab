package rules

import "testing"

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
`

func TestParse(t *testing.T) {
	rules, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
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
}
