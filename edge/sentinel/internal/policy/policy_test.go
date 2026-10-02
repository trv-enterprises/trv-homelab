package policy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

func TestQuietHoursWindow(t *testing.T) {
	loc := time.UTC
	q := QuietHours{Enabled: true, Start: "23:00", End: "07:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 1, h, m, 0, 0, loc) }
	if !q.ActiveAt(at(23, 30), loc) || !q.ActiveAt(at(2, 0), loc) || !q.ActiveAt(at(6, 59), loc) {
		t.Fatal("midnight-crossing window should be active late night")
	}
	if q.ActiveAt(at(7, 0), loc) || q.ActiveAt(at(12, 0), loc) || q.ActiveAt(at(22, 59), loc) {
		t.Fatal("midnight-crossing window should be inactive by day")
	}
	day := QuietHours{Enabled: true, Start: "09:00", End: "17:00"}
	if !day.ActiveAt(at(12, 0), loc) || day.ActiveAt(at(20, 0), loc) {
		t.Fatal("same-day window wrong")
	}
	if (QuietHours{Enabled: false, Start: "00:00", End: "23:59"}).ActiveAt(at(12, 0), loc) {
		t.Fatal("disabled window must never be active")
	}
	if (QuietHours{Enabled: true, Start: "25:00", End: "07:00"}).Validate() == nil {
		t.Fatal("bad hour accepted")
	}
}

func TestDecide(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	d := NewDecider(st, time.UTC)

	sensor := model.Alert{ID: "1", Kind: model.KindSensor, Rule: "garage", Severity: "warning", Status: model.StatusActive}
	if !d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: sensor}).Push {
		t.Fatal("first sighting should push")
	}
	rep := sensor
	rep.RepeatCount = 1
	if d.Decide(ctx, model.Event{Type: "updated", Alert: rep}).Push {
		t.Fatal("warning repeat must not push under default policy")
	}
	_ = st.SetPolicy(ctx, "garage", store.Policy{Repeats: "all"})
	if !d.Decide(ctx, model.Event{Type: "updated", Alert: rep}).Push {
		t.Fatal("repeat should push with repeats=all")
	}
	resolved := sensor
	resolved.Status = model.StatusResolved
	if d.Decide(ctx, model.Event{Type: "updated", Alert: resolved}).Push {
		t.Fatal("resolved must not push")
	}

	_ = st.SetMute(ctx, "garage", nil)
	if dec := d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: sensor}); dec.Push || dec.Reason != "source muted" {
		t.Fatalf("mute ignored: %+v", dec)
	}
	_ = st.ClearMute(ctx, "garage")

	cam := model.Alert{ID: "2", Kind: model.KindCamera, Rule: "frigate_driveway", Severity: "warning", Status: model.StatusActive,
		Frigate: &model.Frigate{Objects: []string{"car"}}}
	_ = st.SetPolicy(ctx, "frigate_driveway", store.Policy{Repeats: "none", Objects: []string{"person"}})
	if d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: cam}).Push {
		t.Fatal("car should be filtered when policy wants person")
	}
	cam.Frigate.Objects = []string{"car", "Person"}
	if !d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: cam}).Push {
		t.Fatal("person should pass the filter (case-insensitive)")
	}

	// quiet hours: always-on window, then exemption
	_ = d.SetQuietHours(ctx, QuietHours{Enabled: true, Start: "00:00", End: "23:59"})
	if dec := d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: cam}); dec.Push || dec.Reason != "quiet hours" {
		t.Fatalf("quiet hours ignored: %+v", dec)
	}
	_ = st.SetPolicy(ctx, "frigate_driveway", store.Policy{Repeats: "none", Objects: []string{"person"}, AlwaysNotify: true})
	if !d.Decide(ctx, model.Event{Type: "created", Created: true, Alert: cam}).Push {
		t.Fatal("always_notify should bypass quiet hours")
	}
}
