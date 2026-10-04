// Package store is the SQLite persistence layer. One file, WAL mode, a single
// connection: sentinel is a single process with modest write rates, and one
// connection sidesteps SQLITE_BUSY entirely.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate adds columns that CREATE TABLE IF NOT EXISTS cannot add to an
// existing file. Each entry is idempotent: skipped when the column exists.
func migrate(db *sql.DB) error {
	adds := []struct{ table, name, ddl string }{
		{"alerts", "link", `ALTER TABLE alerts ADD COLUMN link TEXT NOT NULL DEFAULT ''`},
		{"alerts", "dashboard_id", `ALTER TABLE alerts ADD COLUMN dashboard_id TEXT NOT NULL DEFAULT ''`},
		{"alerts", "dashboard_vars", `ALTER TABLE alerts ADD COLUMN dashboard_vars TEXT NOT NULL DEFAULT '{}'`},
		{"alerts", "store_name", `ALTER TABLE alerts ADD COLUMN store_name TEXT NOT NULL DEFAULT ''`},
		{"alerts", "subtitle", `ALTER TABLE alerts ADD COLUMN subtitle TEXT NOT NULL DEFAULT ''`},
		{"policies", "passive", `ALTER TABLE policies ADD COLUMN passive INTEGER NOT NULL DEFAULT 0`},
		{"devices", "person", `ALTER TABLE devices ADD COLUMN person TEXT NOT NULL DEFAULT ''`},
	}
	have := map[string]map[string]bool{}
	for _, a := range adds {
		if have[a.table] == nil {
			cols, err := columns(db, a.table)
			if err != nil {
				return err
			}
			have[a.table] = cols
		}
		if have[a.table][a.name] {
			continue
		}
		if _, err := db.Exec(a.ddl); err != nil {
			return fmt.Errorf("add column %s.%s: %w", a.table, a.name, err)
		}
	}
	return nil
}

// columns returns the column names of a table. table is one of our own
// constants, never input: PRAGMA takes no bind parameters.
func columns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		have[name] = true
	}
	return have, rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }

// ---- alerts ---------------------------------------------------------------

const alertCols = `id, kind, status, severity, title, body, rule, device, camera,
	review_id, event_ids, objects, thumb_path, start_ts, end_ts,
	started_at, updated_at, resolved_at, repeat_count, raw,
	link, dashboard_id, dashboard_vars, store_name, subtitle`

type scanner interface{ Scan(dest ...any) error }

func scanAlert(row scanner) (model.Alert, error) {
	var a model.Alert
	var reviewID, resolvedAt sql.NullString
	var eventIDs, objects, thumbPath, startedAt, updatedAt string
	var startTS, endTS float64
	var link, dashboardID, dashboardVars, storeName, subtitle string
	err := row.Scan(&a.ID, &a.Kind, &a.Status, &a.Severity, &a.Title, &a.Body, &a.Rule, &a.Device, &a.Camera,
		&reviewID, &eventIDs, &objects, &thumbPath, &startTS, &endTS,
		&startedAt, &updatedAt, &resolvedAt, &a.RepeatCount, &a.Raw,
		&link, &dashboardID, &dashboardVars, &storeName, &subtitle)
	if err != nil {
		return a, err
	}
	a.Link = link
	if a.Kind == model.KindDashboard {
		d := &model.Dashboard{AlertID: reviewID.String, RuleName: a.Device, Store: storeName, Subtitle: subtitle, DashboardID: dashboardID}
		_ = json.Unmarshal([]byte(dashboardVars), &d.DashboardVars)
		a.Dashboard = d
	}
	a.StartedAt = parseTime(startedAt)
	a.UpdatedAt = parseTime(updatedAt)
	if resolvedAt.Valid {
		t := parseTime(resolvedAt.String)
		a.ResolvedAt = &t
	}
	if a.Kind == model.KindCamera {
		f := &model.Frigate{ReviewID: reviewID.String, ThumbPath: thumbPath, StartTime: startTS, EndTime: endTS}
		_ = json.Unmarshal([]byte(eventIDs), &f.EventIDs)
		_ = json.Unmarshal([]byte(objects), &f.Objects)
		if f.EventIDs == nil {
			f.EventIDs = []string{}
		}
		if f.Objects == nil {
			f.Objects = []string{}
		}
		a.Frigate = f
	}
	return a, nil
}

func (s *Store) Get(ctx context.Context, id string) (model.Alert, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+alertCols+` FROM alerts WHERE id = ?`, id)
	a, err := scanAlert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// ListFilter narrows List. Zero values mean "no filter".
type ListFilter struct {
	Status string
	Kind   string
	Source string    // the alert's rule: a Marshal rule name, frigate_<camera>, dashboard_<slug>
	Since  time.Time // updated_at > Since
	// VisibleTo leaves out alerts hidden from this person (see SetHidden).
	// Empty lists everything.
	VisibleTo string
	// Before continues a listing: only alerts that sort after this cursor.
	Before Cursor
	Limit  int
}

// Cursor is a position in the newest-updated-first order. The id breaks
// ties, so a page boundary that falls between two alerts updated in the same
// instant neither repeats nor skips one.
type Cursor struct {
	UpdatedAt time.Time
	ID        string
}

func (c Cursor) IsZero() bool { return c.UpdatedAt.IsZero() }

// String is the opaque form handed to clients.
func (c Cursor) String() string {
	if c.IsZero() {
		return ""
	}
	return fmtTime(c.UpdatedAt) + "|" + c.ID
}

// ParseCursor reads what String wrote.
func ParseCursor(v string) (Cursor, error) {
	ts, id, ok := strings.Cut(v, "|")
	if !ok || id == "" {
		return Cursor{}, fmt.Errorf("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Cursor{}, fmt.Errorf("bad cursor")
	}
	return Cursor{UpdatedAt: t, ID: id}, nil
}

// CursorAfter is the cursor that continues a listing past a.
func CursorAfter(a model.Alert) Cursor { return Cursor{UpdatedAt: a.UpdatedAt, ID: a.ID} }

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// List returns alerts newest-updated first.
func (s *Store) List(ctx context.Context, f ListFilter) ([]model.Alert, error) {
	var where []string
	var args []any
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Source != "" {
		where = append(where, "rule = ?")
		args = append(args, f.Source)
	}
	if !f.Since.IsZero() {
		where = append(where, "updated_at > ?")
		args = append(args, fmtTime(f.Since))
	}
	if !f.Before.IsZero() {
		at := fmtTime(f.Before.UpdatedAt)
		where = append(where, "(updated_at < ? OR (updated_at = ? AND id < ?))")
		args = append(args, at, at, f.Before.ID)
	}
	if f.VisibleTo != "" {
		where = append(where, "NOT EXISTS (SELECT 1 FROM hidden_alerts h WHERE h.alert_id = alerts.id AND h.person = ?)")
		args = append(args, f.VisibleTo)
	}
	q := `SELECT ` + alertCols + ` FROM alerts`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY updated_at DESC, id DESC"
	limit := f.Limit
	if limit <= 0 || limit > maxListLimit {
		limit = defaultListLimit
	}
	q += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListLimit is the page size List uses for a requested limit.
func ListLimit(requested int) int {
	if requested <= 0 || requested > maxListLimit {
		return defaultListLimit
	}
	return requested
}

// ---- per-person history ---------------------------------------------------

// SetHidden records that an alert is not part of a person's own history.
func (s *Store) SetHidden(ctx context.Context, alertID, person, reason string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO hidden_alerts (alert_id, person, reason) VALUES (?, ?, ?)
		ON CONFLICT(alert_id, person) DO UPDATE SET reason = excluded.reason`, alertID, person, reason)
	return err
}

// ClearHidden makes an alert part of a person's history again (it reached
// them after all: a later repeat pushed).
func (s *Store) ClearHidden(ctx context.Context, alertID, person string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM hidden_alerts WHERE alert_id = ? AND person = ?`, alertID, person)
	return err
}

// IsHidden reports whether an alert is left out of a person's history.
func (s *Store) IsHidden(ctx context.Context, alertID, person string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hidden_alerts WHERE alert_id = ? AND person = ?`, alertID, person).Scan(&n)
	return n > 0, err
}

// SensorMsg is the normalized Marshal alert the ingest layer hands over.
type SensorMsg struct {
	Rule     string
	Device   string
	Severity string
	Message  string
	At       time.Time
	Raw      string
}

// UpsertSensor applies a Marshal "new" or "repeat": it updates the active
// alert for the rule if one exists, otherwise creates one. created reports
// which happened. A "repeat" with no active row creates, because sentinel
// may simply have missed the "new".
func (s *Store) UpsertSensor(ctx context.Context, m SensorMsg, repeat bool) (model.Alert, bool, error) {
	now := fmtTime(m.At)
	row := s.db.QueryRowContext(ctx, `SELECT id FROM alerts WHERE kind = ? AND rule = ? AND status = ?`,
		model.KindSensor, m.Rule, model.StatusActive)
	var id string
	err := row.Scan(&id)
	switch {
	case err == nil:
		inc := 0
		if repeat {
			inc = 1
		}
		_, err = s.db.ExecContext(ctx, `UPDATE alerts SET body = ?, severity = ?, updated_at = ?,
			repeat_count = repeat_count + ?, raw = ? WHERE id = ?`,
			m.Message, m.Severity, now, inc, m.Raw, id)
		if err != nil {
			return model.Alert{}, false, err
		}
		a, err := s.Get(ctx, id)
		return a, false, err
	case errors.Is(err, sql.ErrNoRows):
		id = uuid.NewString()
		title := m.Device
		if title == "" {
			title = m.Rule
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO alerts
			(id, kind, status, severity, title, body, rule, device, started_at, updated_at, repeat_count, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
			id, model.KindSensor, model.StatusActive, m.Severity, title, m.Message, m.Rule, m.Device, now, now, m.Raw)
		if err != nil {
			return model.Alert{}, false, err
		}
		a, err := s.Get(ctx, id)
		return a, true, err
	default:
		return model.Alert{}, false, err
	}
}

// ResolveByRule marks the active sensor alert for rule resolved. ok is false
// when there was none.
func (s *Store) ResolveByRule(ctx context.Context, rule string, at time.Time, raw string) (model.Alert, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id FROM alerts WHERE kind = ? AND rule = ? AND status = ?`,
		model.KindSensor, rule, model.StatusActive)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Alert{}, false, nil
		}
		return model.Alert{}, false, err
	}
	a, err := s.setStatus(ctx, id, model.StatusResolved, at, raw)
	return a, err == nil, err
}

// Resolve marks any alert resolved/ended by id (human action from the app).
func (s *Store) Resolve(ctx context.Context, id string, at time.Time) (model.Alert, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return a, err
	}
	status := model.StatusResolved
	if a.Kind == model.KindCamera {
		status = model.StatusEnded
	}
	return s.setStatus(ctx, id, status, at, "")
}

func (s *Store) setStatus(ctx context.Context, id, status string, at time.Time, raw string) (model.Alert, error) {
	now := fmtTime(at)
	q := `UPDATE alerts SET status = ?, updated_at = ?, resolved_at = ? WHERE id = ?`
	args := []any{status, now, now, id}
	if raw != "" {
		q = `UPDATE alerts SET status = ?, updated_at = ?, resolved_at = ?, raw = ? WHERE id = ?`
		args = []any{status, now, now, raw, id}
	}
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return model.Alert{}, err
	}
	return s.Get(ctx, id)
}

// ReviewMsg is the normalized Frigate review the ingest layer hands over.
type ReviewMsg struct {
	ReviewID  string
	Camera    string
	Severity  string // frigate's: "alert" | "detection"
	ThumbPath string
	EventIDs  []string
	Objects   []string
	Zones     []string
	StartTime float64
	EndTime   float64
	Ended     bool
	At        time.Time
	Raw       string
}

// UpsertReview applies a Frigate review message. A row is created the first
// time a review is seen with severity "alert" (whatever the message type,
// since Frigate promotes detections on "update"). Later messages merge
// detections/objects and "end" closes it. created reports a first sighting;
// ok is false when the message was ignored (detection-only review never
// promoted).
func (s *Store) UpsertReview(ctx context.Context, m ReviewMsg) (a model.Alert, created bool, ok bool, err error) {
	now := fmtTime(m.At)
	row := s.db.QueryRowContext(ctx, `SELECT id, event_ids, objects FROM alerts WHERE review_id = ?`, m.ReviewID)
	var id, eventIDs, objects string
	err = row.Scan(&id, &eventIDs, &objects)
	switch {
	case err == nil:
		var have, haveObj []string
		_ = json.Unmarshal([]byte(eventIDs), &have)
		_ = json.Unmarshal([]byte(objects), &haveObj)
		mergedIDs, _ := json.Marshal(union(have, m.EventIDs))
		mergedObj, _ := json.Marshal(union(haveObj, m.Objects))
		status := model.StatusActive
		var resolved any
		if m.Ended {
			status = model.StatusEnded
			resolved = now
		}
		_, err = s.db.ExecContext(ctx, `UPDATE alerts SET event_ids = ?, objects = ?, body = ?, end_ts = ?,
			status = CASE WHEN status = 'ended' THEN status ELSE ? END,
			resolved_at = COALESCE(resolved_at, ?), updated_at = ?, raw = ? WHERE id = ?`,
			string(mergedIDs), string(mergedObj), cameraBody(union(haveObj, m.Objects), m.Zones), m.EndTime,
			status, resolved, now, m.Raw, id)
		if err != nil {
			return model.Alert{}, false, false, err
		}
		a, err = s.Get(ctx, id)
		return a, false, true, err
	case errors.Is(err, sql.ErrNoRows):
		if m.Severity != "alert" {
			return model.Alert{}, false, false, nil
		}
		id = uuid.NewString()
		ids, _ := json.Marshal(nonNil(m.EventIDs))
		objs, _ := json.Marshal(nonNil(m.Objects))
		status := model.StatusActive
		var resolved any
		if m.Ended {
			status = model.StatusEnded
			resolved = now
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO alerts
			(id, kind, status, severity, title, body, rule, device, camera, review_id, event_ids, objects,
			 thumb_path, start_ts, end_ts, started_at, updated_at, resolved_at, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, model.KindCamera, status, model.SeverityWarning, cameraTitle(m.Camera, m.Objects),
			cameraBody(m.Objects, m.Zones), "frigate_"+m.Camera, m.Camera, m.Camera, m.ReviewID,
			string(ids), string(objs), m.ThumbPath, m.StartTime, m.EndTime, now, now, resolved, m.Raw)
		if err != nil {
			return model.Alert{}, false, false, err
		}
		a, err = s.Get(ctx, id)
		return a, true, true, err
	default:
		return model.Alert{}, false, false, err
	}
}

// MarkStale flips active sensor alerts for rule to stale when their
// updated_at is older than before. Returns the affected alerts.
func (s *Store) MarkStale(ctx context.Context, rule string, before, at time.Time) ([]model.Alert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM alerts WHERE kind = ? AND rule = ? AND status = ? AND updated_at < ?`,
		model.KindSensor, rule, model.StatusActive, fmtTime(before))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []model.Alert
	for _, id := range ids {
		a, err := s.setStatus(ctx, id, model.StatusStale, at, "")
		if err != nil {
			return out, err
		}
		out = append(out, a)
	}
	return out, nil
}

// ActiveSensorRules returns rule -> last updated_at for every active sensor alert.
func (s *Store) ActiveSensorRules(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rule, MAX(updated_at) FROM alerts WHERE kind = ? AND status = ? GROUP BY rule`,
		model.KindSensor, model.StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var rule, ts string
		if err := rows.Scan(&rule, &ts); err != nil {
			return nil, err
		}
		out[rule] = parseTime(ts)
	}
	return out, rows.Err()
}

// Prune deletes alerts not updated since before. Returns rows removed.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE updated_at < ?`, fmtTime(before))
	if err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM hidden_alerts WHERE alert_id NOT IN (SELECT id FROM alerts)`); err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- devices --------------------------------------------------------------

func (s *Store) UpsertDevice(ctx context.Context, d model.Device) error {
	now := fmtTime(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices (token, env, name, app_version, created_at, last_seen, person)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET env = excluded.env, name = excluded.name,
			app_version = excluded.app_version, last_seen = excluded.last_seen, person = excluded.person`,
		d.Token, d.Env, d.Name, d.AppVersion, now, now, d.Person)
	return err
}

func (s *Store) ListDevices(ctx context.Context) ([]model.Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT token, env, name, app_version, created_at, last_seen, person FROM devices ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Device{}
	for rows.Next() {
		var d model.Device
		var c, l string
		if err := rows.Scan(&d.Token, &d.Env, &d.Name, &d.AppVersion, &c, &l, &d.Person); err != nil {
			return nil, err
		}
		d.CreatedAt, d.LastSeen = parseTime(c), parseTime(l)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDevice(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE token = ?`, token)
	return err
}

// ---- mutes (phase 3 plumbing) -------------------------------------------

func (s *Store) IsMuted(ctx context.Context, person, rule string, now time.Time) (bool, error) {
	var until sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT until FROM person_mutes WHERE person = ? AND rule = ?`, person, rule).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !until.Valid {
		return true, nil // indefinite
	}
	return parseTime(until.String).After(now), nil
}

// ---- helpers --------------------------------------------------------------

func fmtTime(t time.Time) string   { return t.UTC().Format(time.RFC3339Nano) }
func parseTime(s string) time.Time { t, _ := time.Parse(time.RFC3339Nano, s); return t }

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range append(append([]string{}, a...), b...) {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func cameraTitle(camera string, objects []string) string {
	label := "Motion"
	if len(objects) > 0 {
		label = strings.ToUpper(objects[0][:1]) + objects[0][1:]
	}
	return label + " on " + strings.ReplaceAll(camera, "_", " ")
}

func cameraBody(objects, zones []string) string {
	parts := []string{}
	if len(objects) > 0 {
		parts = append(parts, strings.Join(objects, ", "))
	}
	if len(zones) > 0 {
		parts = append(parts, "in "+strings.Join(zones, ", "))
	}
	if len(parts) == 0 {
		return "Camera alert"
	}
	return strings.Join(parts, " ")
}

// SetMute mutes a rule for a person until `until` (nil = indefinitely).
func (s *Store) SetMute(ctx context.Context, person, rule string, until *time.Time) error {
	var u any
	if until != nil {
		u = fmtTime(*until)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO person_mutes (person, rule, until) VALUES (?, ?, ?)
		ON CONFLICT(person, rule) DO UPDATE SET until = excluded.until`, person, rule, u)
	return err
}

// ClearMute removes a person's mute of a rule.
func (s *Store) ClearMute(ctx context.Context, person, rule string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM person_mutes WHERE person = ? AND rule = ?`, person, rule)
	return err
}

// Mute is one row of the mutes table.
type Mute struct {
	Rule  string
	Until *time.Time // nil = indefinite
}

// Mutes returns every mute of a person's that is still in force at now.
// Expired rows are deleted on the way through.
func (s *Store) Mutes(ctx context.Context, person string, now time.Time) (map[string]Mute, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rule, until FROM person_mutes WHERE person = ?`, person)
	if err != nil {
		return nil, err
	}
	out := map[string]Mute{}
	var expired []string
	for rows.Next() {
		var rule string
		var until sql.NullString
		if err := rows.Scan(&rule, &until); err != nil {
			rows.Close()
			return nil, err
		}
		m := Mute{Rule: rule}
		if until.Valid {
			t := parseTime(until.String)
			if !t.After(now) {
				expired = append(expired, rule)
				continue
			}
			m.Until = &t
		}
		out[rule] = m
	}
	rows.Close()
	for _, r := range expired {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM person_mutes WHERE person = ? AND rule = ?`, person, r)
	}
	return out, nil
}

// ---- policies & settings (0.3.0) ----------------------------------------

// Policy is a source's notification policy. Zero value is not the default;
// use DefaultPolicy.
type Policy struct {
	Repeats      string   `json:"repeats"` // none | critical | all
	Objects      []string `json:"objects"`
	AlwaysNotify bool     `json:"always_notify"`
	// Passive (0.6.0) changes how a push is delivered, never whether: it
	// goes to Notification Center with no banner, sound or screen wake.
	Passive bool `json:"passive"`
}

func DefaultPolicy() Policy { return Policy{Repeats: "critical", Objects: []string{}} }

// Policies returns every policy a person has stored, keyed by source id.
func (s *Store) Policies(ctx context.Context, person string) (map[string]Policy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, repeats, objects, always_notify, passive FROM person_policies WHERE person = ?`, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Policy{}
	for rows.Next() {
		var src, repeats, objects string
		var always, passive int
		if err := rows.Scan(&src, &repeats, &objects, &always, &passive); err != nil {
			return nil, err
		}
		p := Policy{Repeats: repeats, AlwaysNotify: always == 1, Passive: passive == 1}
		_ = json.Unmarshal([]byte(objects), &p.Objects)
		if p.Objects == nil {
			p.Objects = []string{}
		}
		out[src] = p
	}
	return out, rows.Err()
}

// GetPolicy returns a person's stored policy for a source, or the default.
func (s *Store) GetPolicy(ctx context.Context, person, source string) (Policy, error) {
	all, err := s.Policies(ctx, person)
	if err != nil {
		return DefaultPolicy(), err
	}
	if p, ok := all[source]; ok {
		return p, nil
	}
	return DefaultPolicy(), nil
}

// SetPolicy stores a person's full policy for a source.
func (s *Store) SetPolicy(ctx context.Context, person, source string, p Policy) error {
	objs, _ := json.Marshal(nonNil(p.Objects))
	always, passive := 0, 0
	if p.AlwaysNotify {
		always = 1
	}
	if p.Passive {
		passive = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO person_policies (person, source, repeats, objects, always_notify, passive) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(person, source) DO UPDATE SET repeats = excluded.repeats, objects = excluded.objects,
			always_notify = excluded.always_notify, passive = excluded.passive`,
		person, source, p.Repeats, string(objs), always, passive)
	return err
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetPersonSetting reads one of a person's own settings.
func (s *Store) GetPersonSetting(ctx context.Context, person, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM person_settings WHERE person = ? AND key = ?`, person, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// SetPersonSetting stores one of a person's own settings. An empty value
// removes it.
func (s *Store) SetPersonSetting(ctx context.Context, person, key, value string) error {
	if value == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM person_settings WHERE person = ? AND key = ?`, person, key)
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO person_settings (person, key, value) VALUES (?, ?, ?)
		ON CONFLICT(person, key) DO UPDATE SET value = excluded.value`, person, key, value)
	return err
}

// ---- adoption (0.7.0) -----------------------------------------------------

const adoptedKey = "people_adopted"

// QuietHoursKey is the settings key quiet hours live under, both in the
// single-person settings table from before 0.7.0 and per person since.
const QuietHoursKey = "quiet_hours"

// OutpostUserKey is the person setting holding their Outpost (dashboard)
// user id, which dashboard links carry so the browser opens signed in as
// them. Treat the value like a credential: never log it.
const OutpostUserKey = "outpost_user_id"

// Adopt hands everything the single-person server had to owner: the mutes,
// the policies, quiet hours, and every registered phone. It runs once (the
// marker in settings records for whom) and leaves the old tables alone, so a
// rollback to an earlier version finds its data unchanged. Devices with no
// person are claimed on every start, which is harmless: only a phone
// registered by an older server can lack one.
func (s *Store) Adopt(ctx context.Context, owner string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE devices SET person = ? WHERE person = ''`, owner); err != nil {
		return err
	}
	if _, done, err := s.GetSetting(ctx, adoptedKey); err != nil || done {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`INSERT OR IGNORE INTO person_mutes (person, rule, until) SELECT ?, rule, until FROM mutes`,
		`INSERT OR IGNORE INTO person_policies (person, source, repeats, objects, always_notify, passive)
			SELECT ?, source, repeats, objects, always_notify, passive FROM policies`,
		`INSERT OR IGNORE INTO person_settings (person, key, value) SELECT ?, key, value FROM settings WHERE key = '` + QuietHoursKey + `'`,
	} {
		if _, err := tx.ExecContext(ctx, q, owner); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)`, adoptedKey, owner); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- dashboard alerts (0.4.0) ---------------------------------------------

// DashboardMsg is a normalized dashboard alert (from SSE or the unseen list).
type DashboardMsg struct {
	ExternalID    string // the dashboard's alert id
	SourceID      string // sentinel source id: dashboard_<slug>
	RuleName      string
	Store         string
	Title         string
	Subtitle      string
	Severity      string // already mapped to sentinel's scale
	DashboardID   string
	DashboardVars map[string]string
	Link          string
	FiredAt       time.Time
	Raw           string
}

// UpsertDashboard inserts an active alert for a new dashboard alert id, or
// refreshes updated_at/raw for one already known (status untouched).
func (s *Store) UpsertDashboard(ctx context.Context, m DashboardMsg) (model.Alert, bool, error) {
	now := fmtTime(time.Now())
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM alerts WHERE review_id = ?`, m.ExternalID).Scan(&id)
	switch {
	case err == nil:
		if _, err := s.db.ExecContext(ctx, `UPDATE alerts SET updated_at = ?, raw = ?, link = CASE WHEN ? != '' THEN ? ELSE link END WHERE id = ?`,
			now, m.Raw, m.Link, m.Link, id); err != nil {
			return model.Alert{}, false, err
		}
		a, err := s.Get(ctx, id)
		return a, false, err
	case errors.Is(err, sql.ErrNoRows):
		id = uuid.NewString()
		vars, _ := json.Marshal(m.DashboardVars)
		if m.DashboardVars == nil {
			vars = []byte("{}")
		}
		started := now
		if !m.FiredAt.IsZero() {
			started = fmtTime(m.FiredAt)
		}
		body := m.Subtitle
		if body == "" {
			body = m.RuleName
			if m.Store != "" {
				body += " on " + m.Store
			}
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO alerts
			(id, kind, status, severity, title, body, rule, device, review_id, started_at, updated_at, raw,
			 link, dashboard_id, dashboard_vars, store_name, subtitle)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, model.KindDashboard, model.StatusActive, m.Severity, m.Title, body, m.SourceID, m.RuleName, m.ExternalID,
			started, now, m.Raw, m.Link, m.DashboardID, string(vars), m.Store, m.Subtitle)
		if err != nil {
			return model.Alert{}, false, err
		}
		a, err := s.Get(ctx, id)
		return a, true, err
	default:
		return model.Alert{}, false, err
	}
}

// ActiveExternalIDs maps external id -> alert id for active alerts of a kind.
func (s *Store) ActiveExternalIDs(ctx context.Context, kind string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT review_id, id FROM alerts WHERE kind = ? AND status = ? AND review_id IS NOT NULL`, kind, model.StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ext, id string
		if err := rows.Scan(&ext, &id); err != nil {
			return nil, err
		}
		out[ext] = id
	}
	return out, rows.Err()
}

// RecentSourceNames returns distinct (rule, device) pairs for a kind, newest
// first, used to list dashboard sources that have fired.
func (s *Store) RecentSourceNames(ctx context.Context, kind string, limit int) ([][2]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rule, device, MAX(updated_at) AS u FROM alerts WHERE kind = ? GROUP BY rule, device ORDER BY u DESC LIMIT ?`, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var rule, device, u string
		if err := rows.Scan(&rule, &device, &u); err != nil {
			return nil, err
		}
		out = append(out, [2]string{rule, device})
	}
	return out, rows.Err()
}
