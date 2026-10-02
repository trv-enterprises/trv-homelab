// Package dashboard pulls ts-store alerts from the dashboard, which is their
// system of record (the bell, the dashboard link). The dashboard already
// streams every alert over SSE and lists the unseen ones over REST; sentinel
// subscribes with a view-only API key and needs no dashboard change.
//
// The SSE stream has no replay, so the runner backfills from the unseen list
// on start and after every reconnect, and reconciles every few minutes:
// an alert sentinel still has active that the dashboard no longer lists as
// unseen was dismissed there, and is resolved here too.
package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

// Alert is the dashboard's wire shape, shared by the SSE payload and the
// REST list (the list carries more fields; these are the ones used).
type Alert struct {
	ID            string            `json:"id"`
	Severity      string            `json:"severity"` // info | warning | error
	Title         string            `json:"title"`
	Subtitle      string            `json:"subtitle"`
	Source        string            `json:"source"` // ts-store store name
	RuleName      string            `json:"rule_name"`
	FiredAt       time.Time         `json:"fired_at"`
	DashboardID   string            `json:"dashboard_id"`
	DashboardVars map[string]string `json:"dashboard_vars"`
	Seen          bool              `json:"seen"`
	Pinned        bool              `json:"pinned"`
}

type Config struct {
	BaseURL    string // API base
	APIKey     string
	PublicURL  string // what the phone opens
	LinkUserID string // appended as user_id
	MarkSeen   bool
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) req(ctx context.Context, method, path string) (*http.Request, error) {
	r, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	r.Header.Set("Accept", "application/json")
	return r, nil
}

// ListUnseen returns the dashboard's unseen-or-pinned alerts (its only list).
func (c *Client) ListUnseen(ctx context.Context) ([]Alert, error) {
	r, err := c.req(ctx, http.MethodGet, "/api/alerts")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/alerts: %s", resp.Status)
	}
	var body struct {
		Alerts []Alert `json:"alerts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Alerts, nil
}

// MarkSeen dismisses an alert in the dashboard (global, idempotent).
func (c *Client) MarkSeen(ctx context.Context, id string) error {
	r, err := c.req(ctx, http.MethodPost, "/api/alerts/"+url.PathEscape(id)+"/seen")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("POST seen: %s", resp.Status)
	}
	return nil
}

// Stream reads the SSE feed until ctx ends or the connection drops. onAlert
// is called for each `alert` event; onOpen when the stream is established.
func (c *Client) Stream(ctx context.Context, onOpen func(), onAlert func(Alert)) error {
	r, err := c.req(ctx, http.MethodGet, "/api/events/stream")
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "text/event-stream")
	client := &http.Client{} // no timeout: long-lived
	resp, err := client.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("stream: %s", resp.Status)
	}
	onOpen()
	return ParseSSE(ctx, resp.Body, func(event, data string) {
		if event != "alert" {
			return
		}
		var a Alert
		if err := json.Unmarshal([]byte(data), &a); err != nil {
			slog.Warn("dashboard: bad alert event", "error", err)
			return
		}
		onAlert(a)
	})
}

// ParseSSE reads text/event-stream frames and calls fn(event, data) for each.
// Comment lines (": heartbeat") are ignored but count as liveness.
func ParseSSE(ctx context.Context, body io.Reader, fn func(event, data string)) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var event string
	var data []string
	dispatch := func() {
		if len(data) > 0 {
			fn(event, strings.Join(data, "\n"))
		}
		event, data = "", nil
	}
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Text()
		switch {
		case line == "":
			dispatch()
		case strings.HasPrefix(line, ":"):
			// comment / heartbeat
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	dispatch()
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// Link builds the page the phone should open for an alert, or "" if the
// alert has no dashboard. Mirrors the bell's handleOpenDashboard: path
// /view/dashboards/<id> plus var_<name>=<value> for each variable, then the
// user id so a fresh Safari is signed in (legacy GUID auth).
func (c *Client) Link(a Alert) string {
	if a.DashboardID == "" || c.cfg.PublicURL == "" {
		return ""
	}
	q := url.Values{}
	for k, v := range a.DashboardVars {
		if k != "" && v != "" {
			q.Set("var_"+k, v)
		}
	}
	if c.cfg.LinkUserID != "" {
		q.Set("user_id", c.cfg.LinkUserID)
	}
	u := c.cfg.PublicURL + "/view/dashboards/" + url.PathEscape(a.DashboardID)
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SourceID is the sentinel source id for a dashboard rule: dashboard_<slug>.
func SourceID(ruleName string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(ruleName), "-"), "-")
	if slug == "" {
		slug = "unnamed"
	}
	return "dashboard_" + slug
}

// Severity maps the dashboard's scale to sentinel's.
func Severity(s string) string {
	switch strings.ToLower(s) {
	case "error", "critical":
		return model.SeverityCritical
	case "info":
		return model.SeverityInfo
	default:
		return model.SeverityWarning
	}
}

// ToMsg converts a dashboard alert into a store message.
func (c *Client) ToMsg(a Alert, raw string) store.DashboardMsg {
	return store.DashboardMsg{
		ExternalID: a.ID, SourceID: SourceID(a.RuleName), RuleName: a.RuleName, Store: a.Source,
		Title: a.Title, Subtitle: a.Subtitle, Severity: Severity(a.Severity),
		DashboardID: a.DashboardID, DashboardVars: a.DashboardVars, Link: c.Link(a), FiredAt: a.FiredAt, Raw: raw,
	}
}

// Sink receives store changes (the api Hub).
type Sink interface {
	OnEvent(ev model.Event)
}

// Runner owns the stream loop, backfill and reconcile.
type Runner struct {
	client *Client
	store  *store.Store
	sink   Sink

	mu        sync.Mutex
	connected bool
	lastEvent time.Time
	lastErr   string
}

func NewRunner(client *Client, st *store.Store, sink Sink) *Runner {
	return &Runner{client: client, store: st, sink: sink}
}

type Status struct {
	Enabled   bool      `json:"enabled"`
	Connected bool      `json:"connected"`
	LastEvent time.Time `json:"last_event,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

func (r *Runner) Status() Status {
	if r == nil {
		return Status{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return Status{Enabled: true, Connected: r.connected, LastEvent: r.lastEvent, LastError: r.lastErr}
}

// MarkSeenEnabled reports whether app resolves should propagate.
func (r *Runner) MarkSeenEnabled() bool { return r != nil && r.client.cfg.MarkSeen }

// MarkSeen propagates an app-side resolve to the dashboard.
func (r *Runner) MarkSeen(ctx context.Context, externalID string) error {
	return r.client.MarkSeen(ctx, externalID)
}

// Run blocks until ctx is done.
func (r *Runner) Run(ctx context.Context) {
	go r.reconcileLoop(ctx)
	backoff := time.Second
	for ctx.Err() == nil {
		r.backfill(ctx)
		err := r.client.Stream(ctx, func() {
			r.mu.Lock()
			r.connected, r.lastErr = true, ""
			r.mu.Unlock()
			backoff = time.Second
			slog.Info("dashboard: stream connected")
		}, func(a Alert) {
			r.handle(ctx, a)
		})
		r.mu.Lock()
		r.connected = false
		if err != nil && ctx.Err() == nil {
			r.lastErr = err.Error()
		}
		r.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		slog.Warn("dashboard: stream ended, reconnecting", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

func (r *Runner) handle(ctx context.Context, a Alert) {
	raw, _ := json.Marshal(a)
	alert, created, err := r.store.UpsertDashboard(ctx, r.client.ToMsg(a, string(raw)))
	if err != nil {
		slog.Error("dashboard: store", "error", err)
		return
	}
	r.mu.Lock()
	r.lastEvent = time.Now()
	r.mu.Unlock()
	typ := "updated"
	if created {
		typ = "created"
	}
	if r.sink != nil {
		r.sink.OnEvent(model.Event{Type: typ, Alert: alert, Created: created})
	}
}

// backfill upserts every alert the dashboard currently lists as unseen.
// Backfilled alerts are created like streamed ones (and push), which is
// the point: an alert that fired while sentinel was down still reaches
// the phone once.
func (r *Runner) backfill(ctx context.Context) {
	bctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	list, err := r.client.ListUnseen(bctx)
	if err != nil {
		slog.Warn("dashboard: backfill failed", "error", err)
		return
	}
	for _, a := range list {
		if a.Seen && !a.Pinned {
			continue
		}
		r.handle(ctx, a)
	}
}

// reconcileLoop resolves alerts the dashboard no longer lists as unseen.
func (r *Runner) reconcileLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Runner) reconcile(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	list, err := r.client.ListUnseen(rctx)
	if err != nil {
		return // keep state; a failed list must not resolve everything
	}
	unseen := map[string]bool{}
	for _, a := range list {
		unseen[a.ID] = true
	}
	active, err := r.store.ActiveExternalIDs(rctx, model.KindDashboard)
	if err != nil {
		return
	}
	for ext, id := range active {
		if unseen[ext] {
			continue
		}
		a, err := r.store.Resolve(rctx, id, time.Now())
		if err != nil {
			continue
		}
		slog.Info("dashboard: resolved (dismissed in dashboard)", "id", id)
		if r.sink != nil {
			r.sink.OnEvent(model.Event{Type: "updated", Alert: a})
		}
	}
}
