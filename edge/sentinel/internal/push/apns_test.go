package push

import (
	"encoding/json"
	"testing"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

// aps returns the "aps" dictionary of the payload build() produces: what
// iOS actually reads to decide how to present the notification.
func aps(t *testing.T, a model.Alert, passive bool) map[string]any {
	t.Helper()
	b, err := json.Marshal((&Pusher{}).build(a, passive))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		APS map[string]any `json:"aps"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.APS
}

func TestInterruptionLevel(t *testing.T) {
	motion := model.Alert{ID: "1", Kind: model.KindSensor, Rule: "nightlight_motion_hall", Severity: "info", Title: "Motion in the hall"}
	garage := model.Alert{ID: "2", Kind: model.KindSensor, Rule: "garage", Severity: "warning"}
	freezer := model.Alert{ID: "3", Kind: model.KindSensor, Rule: "freezer", Severity: model.SeverityCritical}
	camera := model.Alert{ID: "4", Kind: model.KindCamera, Rule: "frigate_driveway", Severity: "warning"}
	dash := model.Alert{ID: "5", Kind: model.KindDashboard, Rule: "dashboard_x", Severity: "warning", Link: "http://d/x"}

	cases := []struct {
		name    string
		alert   model.Alert
		passive bool
		level   string
		sound   bool
	}{
		{"info sensor", motion, false, "active", true},
		{"warning sensor", garage, false, "active", true},
		{"critical sensor", freezer, false, "time-sensitive", true},
		{"camera", camera, false, "time-sensitive", true},
		{"dashboard", dash, false, "active", true},
		{"passive sensor", motion, true, "passive", false},
		// The source's own setting wins over the automatic time-sensitive.
		{"passive critical sensor", freezer, true, "passive", false},
		{"passive camera", camera, true, "passive", false},
		{"passive dashboard", dash, true, "passive", false},
	}
	for _, c := range cases {
		got := aps(t, c.alert, c.passive)
		if got["interruption-level"] != c.level {
			t.Errorf("%s: interruption-level %v want %s", c.name, got["interruption-level"], c.level)
		}
		if _, has := got["sound"]; has != c.sound {
			t.Errorf("%s: sound present=%v want %v", c.name, has, c.sound)
		}
		// Passive or not, the extension must still run (thumbnail) and the
		// alert must still be an alert, not a silent background push.
		if got["mutable-content"] != float64(1) {
			t.Errorf("%s: mutable-content %v", c.name, got["mutable-content"])
		}
		if _, ok := got["alert"]; !ok {
			t.Errorf("%s: no alert", c.name)
		}
		if _, ok := got["content-available"]; ok {
			t.Errorf("%s: content-available set", c.name)
		}
	}
	// The dashboard action category survives a passive delivery.
	if got := aps(t, dash, true); got["category"] != "dashboard" {
		t.Errorf("passive dashboard: category %v", got["category"])
	}
}
