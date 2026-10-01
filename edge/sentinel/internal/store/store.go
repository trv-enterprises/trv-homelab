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
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---- alerts ---------------------------------------------------------------

const alertCols = `id, kind, status, severity, title, body, rule, device, camera,
	review_id, event_ids, objects, thumb_path, start_ts, end_ts,
	started_at, updated_at, resolved_at, repeat_count, raw`

type scanner interface{ Scan(dest ...any) error }

func scanAlert(row scanner) (model.Alert, error) {
	var a model.Alert
	var reviewID, resolvedAt sql.NullString
	var eventIDs, objects, thumbPath, startedAt, updatedAt string
	var startTS, endTS float64
	err := row.Scan(&a.ID, &a.Kind, &a.Status, &a.Severity, &a.Title, &a.Body, &a.Rule, &a.Device, &a.Camera,
		&reviewID, &eventIDs, &objects, &thumbPath, &startTS, &endTS,
		&startedAt, &updatedAt, &resolvedAt, &a.RepeatCount, &a.Raw)
	if err != nil {
		return a, err
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
	Since  time.Time // updated_at > Since
	Limit  int
}

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
	if !f.Since.IsZero() {
		where = append(where, "updated_at > ?")
		args = append(args, fmtTime(f.Since))
	}
	q := `SELECT ` + alertCols + ` FROM alerts`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY updated_at DESC"
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
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
	return res.RowsAffected()
}

// ---- devices --------------------------------------------------------------

func (s *Store) UpsertDevice(ctx context.Context, d model.Device) error {
	now := fmtTime(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices (token, env, name, app_version, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET env = excluded.env, name = excluded.name,
			app_version = excluded.app_version, last_seen = excluded.last_seen`,
		d.Token, d.Env, d.Name, d.AppVersion, now, now)
	return err
}

func (s *Store) ListDevices(ctx context.Context) ([]model.Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT token, env, name, app_version, created_at, last_seen FROM devices ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Device{}
	for rows.Next() {
		var d model.Device
		var c, l string
		if err := rows.Scan(&d.Token, &d.Env, &d.Name, &d.AppVersion, &c, &l); err != nil {
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

func (s *Store) IsMuted(ctx context.Context, rule string, now time.Time) (bool, error) {
	var until sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT until FROM mutes WHERE rule = ?`, rule).Scan(&until)
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
