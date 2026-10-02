// Package policy makes the one decision that matters to the phone: does
// this alert event produce a push? Everything else (storing, streaming,
// listing) happens regardless of what this package says.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

const quietKey = "quiet_hours"

// QuietHours is a daily do-not-disturb window in the server's local zone.
// A window that ends before it starts crosses midnight (23:00 to 07:00).
type QuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // HH:MM
	End     string `json:"end"`   // HH:MM
}

func DefaultQuietHours() QuietHours { return QuietHours{Enabled: false, Start: "23:00", End: "07:00"} }

func parseHM(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("bad time %q, want HH:MM", s)
	}
	return h*60 + m, nil
}

// Validate checks the window.
func (q QuietHours) Validate() error {
	if _, err := parseHM(q.Start); err != nil {
		return err
	}
	if _, err := parseHM(q.End); err != nil {
		return err
	}
	return nil
}

// ActiveAt reports whether t (converted to loc) falls inside the window.
func (q QuietHours) ActiveAt(t time.Time, loc *time.Location) bool {
	if !q.Enabled {
		return false
	}
	start, err1 := parseHM(q.Start)
	end, err2 := parseHM(q.End)
	if err1 != nil || err2 != nil || start == end {
		return false
	}
	lt := t.In(loc)
	now := lt.Hour()*60 + lt.Minute()
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end // crosses midnight
}

// Decider owns the push decision.
type Decider struct {
	store *store.Store
	loc   *time.Location
}

func NewDecider(st *store.Store, loc *time.Location) *Decider {
	if loc == nil {
		loc = time.Local
	}
	return &Decider{store: st, loc: loc}
}

func (d *Decider) Location() *time.Location { return d.loc }

// QuietHours loads the configured window.
func (d *Decider) QuietHours(ctx context.Context) (QuietHours, error) {
	v, ok, err := d.store.GetSetting(ctx, quietKey)
	if err != nil || !ok {
		return DefaultQuietHours(), err
	}
	var q QuietHours
	if err := json.Unmarshal([]byte(v), &q); err != nil {
		return DefaultQuietHours(), nil
	}
	return q, nil
}

func (d *Decider) SetQuietHours(ctx context.Context, q QuietHours) error {
	if err := q.Validate(); err != nil {
		return err
	}
	b, _ := json.Marshal(q)
	return d.store.SetSetting(ctx, quietKey, string(b))
}

// Decision explains a verdict; Reason is for logs and the test endpoint.
type Decision struct {
	Push   bool
	Reason string
}

// Decide applies the ordered rules from the README: pushable event, mute,
// object filter, quiet hours.
func (d *Decider) Decide(ctx context.Context, ev model.Event) Decision {
	a := ev.Alert
	pol, err := d.store.GetPolicy(ctx, a.Rule)
	if err != nil {
		slog.Warn("policy lookup failed; using defaults", "source", a.Rule, "error", err)
		pol = store.DefaultPolicy()
	}

	// 1. pushable event?
	switch {
	case ev.Created:
	case a.Kind == model.KindSensor && ev.Type == "updated" && a.Status == model.StatusActive && a.RepeatCount > 0:
		if pol.Repeats == "none" || (pol.Repeats == "critical" && a.Severity != model.SeverityCritical) {
			return Decision{false, "repeat suppressed by policy " + pol.Repeats}
		}
	default:
		return Decision{false, "not a pushable event"}
	}

	// 2. muted?
	if muted, err := d.store.IsMuted(ctx, a.Rule, time.Now()); err == nil && muted {
		return Decision{false, "source muted"}
	}

	// 3. camera object filter
	if a.Kind == model.KindCamera && len(pol.Objects) > 0 && a.Frigate != nil && len(a.Frigate.Objects) > 0 {
		if !intersects(a.Frigate.Objects, pol.Objects) {
			return Decision{false, "objects " + strings.Join(a.Frigate.Objects, ",") + " not in policy"}
		}
	}

	// 4. quiet hours
	if q, _ := d.QuietHours(ctx); q.ActiveAt(time.Now(), d.loc) && !pol.AlwaysNotify {
		return Decision{false, "quiet hours"}
	}
	return Decision{true, "ok"}
}

func intersects(a, b []string) bool {
	set := map[string]bool{}
	for _, v := range b {
		set[strings.ToLower(v)] = true
	}
	for _, v := range a {
		if set[strings.ToLower(v)] {
			return true
		}
	}
	return false
}

// DecideBool adapts Decide to the push package's interface.
type Adapter struct{ *Decider }

func (a Adapter) Decide(ctx context.Context, ev model.Event) (bool, string) {
	d := a.Decider.Decide(ctx, ev)
	return d.Push, d.Reason
}
