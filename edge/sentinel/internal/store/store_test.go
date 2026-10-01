package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSensorLifecycle(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	msg := SensorMsg{Rule: "garage", Device: "Large Garage Door", Severity: "warning", Message: "open 30m", At: t0}

	a, created, err := s.UpsertSensor(ctx, msg, false)
	if err != nil || !created {
		t.Fatalf("new: created=%v err=%v", created, err)
	}
	if a.Status != model.StatusActive || a.Title != "Large Garage Door" || a.Kind != model.KindSensor {
		t.Fatalf("bad alert %+v", a)
	}

	// second "new" for the same rule updates, no new row
	msg.Message = "open 31m"
	b, created, err := s.UpsertSensor(ctx, msg, false)
	if err != nil || created || b.ID != a.ID || b.Body != "open 31m" {
		t.Fatalf("dup new: created=%v id=%s body=%q err=%v", created, b.ID, b.Body, err)
	}

	// repeat bumps the counter
	msg.At = t0.Add(time.Hour)
	c, _, err := s.UpsertSensor(ctx, msg, true)
	if err != nil || c.RepeatCount != 1 || !c.UpdatedAt.Equal(msg.At) {
		t.Fatalf("repeat: %+v err=%v", c, err)
	}

	// resolve
	r, ok, err := s.ResolveByRule(ctx, "garage", t0.Add(2*time.Hour), "{}")
	if err != nil || !ok || r.Status != model.StatusResolved || r.ResolvedAt == nil {
		t.Fatalf("resolve: ok=%v %+v err=%v", ok, r, err)
	}
	// resolve again: nothing active
	if _, ok, _ := s.ResolveByRule(ctx, "garage", t0, ""); ok {
		t.Fatal("second resolve should find nothing")
	}

	// orphan repeat creates a fresh active alert
	d, created, err := s.UpsertSensor(ctx, msg, true)
	if err != nil || !created || d.ID == a.ID {
		t.Fatalf("orphan repeat: created=%v err=%v", created, err)
	}

	list, err := s.List(ctx, ListFilter{})
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}
	active, _ := s.List(ctx, ListFilter{Status: model.StatusActive})
	if len(active) != 1 || active[0].ID != d.ID {
		t.Fatalf("active filter wrong: %+v", active)
	}
}

func TestReviewPromotionAndMerge(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	t0 := time.Now()

	det := ReviewMsg{ReviewID: "r1", Camera: "driveway", Severity: "detection", EventIDs: []string{"e1"}, Objects: []string{"car"}, StartTime: 1, At: t0}
	if _, created, ok, err := s.UpsertReview(ctx, det); err != nil || created || ok {
		t.Fatalf("detection must be ignored: created=%v ok=%v err=%v", created, ok, err)
	}

	// promoted on update
	det.Severity = "alert"
	det.EventIDs = []string{"e1", "e2"}
	det.Objects = []string{"car", "person"}
	det.Zones = []string{"Street"}
	a, created, ok, err := s.UpsertReview(ctx, det)
	if err != nil || !created || !ok {
		t.Fatalf("promotion: created=%v ok=%v err=%v", created, ok, err)
	}
	if a.Title != "Car on driveway" || a.Body != "car, person in Street" || a.Severity != model.SeverityWarning {
		t.Fatalf("bad camera alert %+v", a)
	}
	if a.Frigate == nil || len(a.Frigate.EventIDs) != 2 {
		t.Fatalf("frigate meta %+v", a.Frigate)
	}

	// later update merges and keeps one row
	det.EventIDs = []string{"e3"}
	b, created, ok, err := s.UpsertReview(ctx, det)
	if err != nil || created || !ok || b.ID != a.ID || len(b.Frigate.EventIDs) != 3 {
		t.Fatalf("merge: created=%v ok=%v ids=%v err=%v", created, ok, b.Frigate.EventIDs, err)
	}

	// end
	det.Ended, det.EndTime = true, 42
	c, _, _, err := s.UpsertReview(ctx, det)
	if err != nil || c.Status != model.StatusEnded || c.Frigate.EndTime != 42 || c.ResolvedAt == nil {
		t.Fatalf("end: %+v err=%v", c, err)
	}
}

func TestStaleAndPrune(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	t0 := time.Now().Add(-48 * time.Hour)
	if _, _, err := s.UpsertSensor(ctx, SensorMsg{Rule: "x", Severity: "info", Message: "m", At: t0}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := s.ActiveSensorRules(ctx)
	if _, ok := rules["x"]; !ok {
		t.Fatal("expected active rule x")
	}
	stale, err := s.MarkStale(ctx, "x", time.Now().Add(-time.Hour), time.Now())
	if err != nil || len(stale) != 1 || stale[0].Status != model.StatusStale {
		t.Fatalf("stale: %+v err=%v", stale, err)
	}
	n, err := s.Prune(ctx, time.Now().Add(time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("prune n=%d err=%v", n, err)
	}
}

func TestDevices(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	d := model.Device{Token: "abc", Env: model.EnvSandbox, Name: "phone"}
	if err := s.UpsertDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Env = model.EnvProduction
	if err := s.UpsertDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListDevices(ctx)
	if len(list) != 1 || list[0].Env != model.EnvProduction {
		t.Fatalf("devices %+v", list)
	}
	if err := s.DeleteDevice(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListDevices(ctx)
	if len(list) != 0 {
		t.Fatal("delete failed")
	}
}
