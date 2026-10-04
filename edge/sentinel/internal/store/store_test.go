package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

const me = "tom"

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

func TestMutes(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	if err := s.SetMute(ctx, me, "forever", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMute(ctx, me, "soon", &future); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMute(ctx, me, "gone", &past); err != nil {
		t.Fatal(err)
	}
	m, err := s.Mutes(ctx, me, now)
	if err != nil || len(m) != 2 || m["forever"].Until != nil || m["soon"].Until == nil {
		t.Fatalf("mutes %+v err=%v", m, err)
	}
	if muted, _ := s.IsMuted(ctx, me, "gone", now); muted {
		t.Fatal("expired mute still active")
	}
	if muted, _ := s.IsMuted(ctx, me, "forever", now); !muted {
		t.Fatal("indefinite mute not active")
	}
	if err := s.ClearMute(ctx, me, "forever"); err != nil {
		t.Fatal(err)
	}
	if muted, _ := s.IsMuted(ctx, me, "forever", now); muted {
		t.Fatal("cleared mute still active")
	}
}

func TestDashboardUpsertAndMigration(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	m := DashboardMsg{ExternalID: "m1", SourceID: "dashboard_high-temp", RuleName: "High Temp", Store: "system-stats",
		Title: "High Temp on Proxmox", Subtitle: "cpu > 80", Severity: "warning", DashboardID: "d1",
		DashboardVars: map[string]string{"host": "a"}, Link: "http://x/view/dashboards/d1?var_host=a", FiredAt: time.Now().Add(-time.Minute)}
	a, created, err := s.UpsertDashboard(ctx, m)
	if err != nil || !created || a.Kind != model.KindDashboard || a.Link == "" || a.Dashboard == nil || a.Dashboard.DashboardVars["host"] != "a" || a.Rule != "dashboard_high-temp" {
		t.Fatalf("insert: %+v created=%v err=%v", a, created, err)
	}
	b, created, err := s.UpsertDashboard(ctx, m)
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("dup: created=%v err=%v", created, err)
	}
	ids, _ := s.ActiveExternalIDs(ctx, model.KindDashboard)
	if ids["m1"] != a.ID {
		t.Fatalf("active ids %+v", ids)
	}
	names, _ := s.RecentSourceNames(ctx, model.KindDashboard, 10)
	if len(names) != 1 || names[0][0] != "dashboard_high-temp" || names[0][1] != "High Temp" {
		t.Fatalf("names %+v", names)
	}
	// no subtitle -> body falls back to rule on store
	c, _, _ := s.UpsertDashboard(ctx, DashboardMsg{ExternalID: "m2", SourceID: "dashboard_x", RuleName: "X", Store: "st", Title: "X", Severity: "info"})
	if c.Body != "X on st" {
		t.Fatalf("body fallback %q", c.Body)
	}
	// reopening the same file runs migrate() on an already-migrated schema
	path := filepath.Join(t.TempDir(), "mig.db")
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
	if _, err := Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

// A database written by 0.5.0 has a policies table without the passive
// column. Opening it must add the column, keep the rows, and default them to
// not passive.
func TestPolicyPassiveAndMigrationFromOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE policies (
		source TEXT PRIMARY KEY,
		repeats TEXT NOT NULL DEFAULT 'critical',
		objects TEXT NOT NULL DEFAULT '[]',
		always_notify INTEGER NOT NULL DEFAULT 0
	); INSERT INTO policies (source, repeats, objects, always_notify) VALUES ('frigate_driveway', 'none', '["person"]', 1);`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open 0.5.0 database: %v", err)
	}
	ctx := context.Background()
	if err := s.Adopt(ctx, me); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPolicy(ctx, me, "frigate_driveway")
	if err != nil || p.Repeats != "none" || len(p.Objects) != 1 || !p.AlwaysNotify || p.Passive {
		t.Fatalf("migrated row: %+v err=%v", p, err)
	}
	if d, _ := s.GetPolicy(ctx, me, "never_set"); d.Passive {
		t.Fatalf("default policy is passive: %+v", d)
	}
	p.Passive = true
	if err := s.SetPolicy(ctx, me, "frigate_driveway", p); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Survives a reopen, and migrate() is a no-op the second time.
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	p, _ = s.GetPolicy(ctx, me, "frigate_driveway")
	if !p.Passive || !p.AlwaysNotify || p.Repeats != "none" {
		t.Fatalf("after reopen: %+v", p)
	}
	p.Passive = false
	_ = s.SetPolicy(ctx, me, "frigate_driveway", p)
	if p, _ = s.GetPolicy(ctx, me, "frigate_driveway"); p.Passive {
		t.Fatalf("passive did not clear: %+v", p)
	}
}

// A 0.6.0 database belongs to nobody in particular. Adopt gives all of it to
// the owner, once, and leaves the old tables as a rollback would need them.
func TestAdoptGivesTheOldServersDataToTheOwner(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	if _, err := s.db.Exec(`
		INSERT INTO mutes (rule, until) VALUES ('garage', NULL), ('kind:camera', ?);
		INSERT INTO policies (source, repeats, objects, always_notify, passive) VALUES ('frigate_doorbell', 'critical', '["person"]', 1, 0), ('hall', 'none', '[]', 0, 1);
		INSERT INTO settings (key, value) VALUES ('quiet_hours', '{"enabled":true,"start":"22:00","end":"06:30"}');
		INSERT INTO devices (token, env, created_at, last_seen) VALUES ('aaaa', 'sandbox', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z');`,
		fmtTime(until)); err != nil {
		t.Fatal(err)
	}
	if err := s.Adopt(ctx, "tom"); err != nil {
		t.Fatal(err)
	}

	mutes, _ := s.Mutes(ctx, "tom", time.Now())
	if len(mutes) != 2 || mutes["garage"].Until != nil || mutes["kind:camera"].Until == nil {
		t.Fatalf("mutes %+v", mutes)
	}
	if p, _ := s.GetPolicy(ctx, "tom", "frigate_doorbell"); !p.AlwaysNotify || len(p.Objects) != 1 || p.Passive {
		t.Fatalf("doorbell %+v", p)
	}
	if p, _ := s.GetPolicy(ctx, "tom", "hall"); !p.Passive || p.Repeats != "none" {
		t.Fatalf("hall %+v", p)
	}
	if v, ok, _ := s.GetPersonSetting(ctx, "tom", QuietHoursKey); !ok || !strings.Contains(v, "22:00") {
		t.Fatalf("quiet hours %q %v", v, ok)
	}
	devs, _ := s.ListDevices(ctx)
	if len(devs) != 1 || devs[0].Person != "tom" {
		t.Fatalf("devices %+v", devs)
	}

	// Someone new starts from defaults, with none of it.
	if m, _ := s.Mutes(ctx, "maria", time.Now()); len(m) != 0 {
		t.Fatalf("maria inherited mutes %+v", m)
	}
	if p, _ := s.GetPolicy(ctx, "maria", "hall"); p.Passive || p.Repeats != "critical" {
		t.Fatalf("maria inherited a policy %+v", p)
	}
	if _, ok, _ := s.GetPersonSetting(ctx, "maria", QuietHoursKey); ok {
		t.Fatal("maria inherited quiet hours")
	}

	// It happens once: what the owner changes afterwards is not overwritten
	// by the old tables on the next start, and a different owner name does
	// not get a second copy.
	_ = s.ClearMute(ctx, "tom", "garage")
	_ = s.SetPolicy(ctx, "tom", "hall", Policy{Repeats: "all"})
	for _, owner := range []string{"tom", "thomas"} {
		if err := s.Adopt(ctx, owner); err != nil {
			t.Fatal(err)
		}
	}
	if muted, _ := s.IsMuted(ctx, "tom", "garage", time.Now()); muted {
		t.Fatal("second Adopt brought back a mute the owner had lifted")
	}
	if p, _ := s.GetPolicy(ctx, "tom", "hall"); p.Passive || p.Repeats != "all" {
		t.Fatalf("second Adopt overwrote a policy: %+v", p)
	}
	if m, _ := s.Mutes(ctx, "thomas", time.Now()); len(m) != 0 {
		t.Fatalf("a renamed owner got a second copy: %+v", m)
	}

	// The old tables are exactly as they were.
	var n int
	_ = s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM mutes) + (SELECT COUNT(*) FROM policies)`).Scan(&n)
	if n != 4 {
		t.Fatalf("old tables changed: %d rows", n)
	}
}

func TestMutesAndPoliciesArePerPerson(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	_ = s.SetMute(ctx, "maria", "garage", nil)
	_ = s.SetPolicy(ctx, "maria", "hall", Policy{Repeats: "critical", Passive: true})
	if muted, _ := s.IsMuted(ctx, "maria", "garage", time.Now()); !muted {
		t.Fatal("maria's mute not stored")
	}
	if muted, _ := s.IsMuted(ctx, "tom", "garage", time.Now()); muted {
		t.Fatal("maria's mute applies to tom")
	}
	if p, _ := s.GetPolicy(ctx, "tom", "hall"); p.Passive {
		t.Fatal("maria's policy applies to tom")
	}
	_ = s.SetMute(ctx, "tom", "garage", nil)
	_ = s.ClearMute(ctx, "maria", "garage")
	if muted, _ := s.IsMuted(ctx, "tom", "garage", time.Now()); !muted {
		t.Fatal("maria unmuting lifted tom's mute")
	}
}

func TestListVisibilityAndCursor(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	// Three alerts updated in the very same instant, so only the id can
	// order them: the case a timestamp-only cursor gets wrong.
	at := fmtTime(time.Now())
	for _, id := range []string{"a1", "a2", "a3"} {
		if _, err := s.db.Exec(`INSERT INTO alerts (id, kind, status, severity, title, body, rule, started_at, updated_at)
			VALUES (?, 'sensor', 'resolved', 'info', 't', 'b', 'hall', ?, ?)`, id, at, at); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	var cur Cursor
	for i := 0; i < 5; i++ {
		page, err := s.List(ctx, ListFilter{Limit: 1, Before: cur})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		got = append(got, page[0].ID)
		// The cursor survives the trip through its wire form.
		if cur, err = ParseCursor(CursorAfter(page[0]).String()); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(got, ",") != "a3,a2,a1" {
		t.Fatalf("paged %v, want each alert exactly once", got)
	}

	_ = s.SetHidden(ctx, "a2", "maria", "source muted")
	ids := func(f ListFilter) string {
		page, _ := s.List(ctx, f)
		var out []string
		for _, a := range page {
			out = append(out, a.ID)
		}
		return strings.Join(out, ",")
	}
	if v := ids(ListFilter{VisibleTo: "maria"}); v != "a3,a1" {
		t.Fatalf("maria sees %s", v)
	}
	if v := ids(ListFilter{VisibleTo: "tom"}); v != "a3,a2,a1" {
		t.Fatalf("tom sees %s", v)
	}
	if v := ids(ListFilter{}); v != "a3,a2,a1" {
		t.Fatalf("everything is %s", v)
	}
	if hidden, _ := s.IsHidden(ctx, "a2", "maria"); !hidden {
		t.Fatal("IsHidden")
	}
	_ = s.ClearHidden(ctx, "a2", "maria")
	if v := ids(ListFilter{VisibleTo: "maria"}); v != "a3,a2,a1" {
		t.Fatalf("after ClearHidden maria sees %s", v)
	}
	// Pruning an alert takes its hidden rows with it.
	_ = s.SetHidden(ctx, "a1", "maria", "source muted")
	if _, err := s.Prune(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM hidden_alerts`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d orphan hidden rows after prune", n)
	}
	for _, bad := range []string{"", "nope", "2026-10-03T00:00:00Z", "|a1", "yesterday|a1"} {
		if _, err := ParseCursor(bad); err == nil {
			t.Errorf("ParseCursor(%q) accepted", bad)
		}
	}
}
