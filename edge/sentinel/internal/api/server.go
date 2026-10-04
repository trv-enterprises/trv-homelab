// Package api is sentinel's HTTP surface: the JSON API the iOS app uses, an
// SSE stream for the foreground app, and a media proxy to Frigate.
//
// Auth is a bearer token per person: the token is what says who is asking,
// and everything a person chooses is theirs alone. Media paths additionally
// accept a signed URL (see media.go) because AVPlayer, AsyncImage and the
// notification extension cannot reliably send headers.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/dashboard"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/frigate"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/ingest"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/policy"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/push"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

type Server struct {
	people  []model.Person
	store   *store.Store
	rules   *rules.Reader
	pusher  *push.Pusher
	engine  *ingest.Engine
	signer  *Signer
	hub     *Hub
	decider *policy.Decider
	cameras *frigate.Cameras
	dash    *dashboard.Runner // nil when the feed is not configured
	proxy   http.Handler
	mux     *http.ServeMux
	started time.Time
}

type Deps struct {
	People     []model.Person // the owner first
	Store      *store.Store
	Rules      *rules.Reader
	Pusher     *push.Pusher
	Engine     *ingest.Engine
	Signer     *Signer
	Hub        *Hub
	Decider    *policy.Decider
	Cameras    *frigate.Cameras
	Dashboard  *dashboard.Runner
	FrigateURL string
}

func New(d Deps) *Server {
	s := &Server{people: d.People, store: d.Store, rules: d.Rules, pusher: d.Pusher, engine: d.Engine,
		signer: d.Signer, hub: d.Hub, decider: d.Decider, cameras: d.Cameras, dash: d.Dashboard, mux: http.NewServeMux(), started: time.Now()}
	s.proxy = newFrigateProxy(d.FrigateURL, d.Signer)

	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /v1/alerts", s.listAlerts)
	s.mux.HandleFunc("GET /v1/alerts/{id}", s.getAlert)
	s.mux.HandleFunc("POST /v1/alerts/{id}/resolve", s.resolveAlert)
	s.mux.HandleFunc("GET /v1/events", s.events)
	s.mux.HandleFunc("GET /v1/media/", s.media)
	s.mux.HandleFunc("POST /v1/devices", s.registerDevice)
	s.mux.HandleFunc("GET /v1/devices", s.listDevices)
	s.mux.HandleFunc("DELETE /v1/devices/{token}", s.deleteDevice)
	s.mux.HandleFunc("POST /v1/push/test", s.testPush)
	s.mux.HandleFunc("GET /v1/rules", s.listRules)
	s.mux.HandleFunc("POST /v1/rules/{name}/mute", s.muteRule)
	s.mux.HandleFunc("DELETE /v1/rules/{name}/mute", s.unmuteRule)
	s.mux.HandleFunc("POST /v1/rules/{name}/enable", s.enableRule)
	s.mux.HandleFunc("GET /v1/sources", s.listSources)
	s.mux.HandleFunc("POST /v1/sources/{id}/mute", s.muteSource)
	s.mux.HandleFunc("DELETE /v1/sources/{id}/mute", s.unmuteSource)
	s.mux.HandleFunc("PUT /v1/sources/{id}/policy", s.putPolicy)
	s.mux.HandleFunc("PUT /v1/sources/{id}/action", s.putAction)
	s.mux.HandleFunc("POST /v1/sources/kind/{kind}/mute", s.muteKind)
	s.mux.HandleFunc("DELETE /v1/sources/kind/{kind}/mute", s.unmuteKind)
	s.mux.HandleFunc("GET /v1/settings/quiet-hours", s.getQuietHours)
	s.mux.HandleFunc("PUT /v1/settings/quiet-hours", s.putQuietHours)
	s.mux.HandleFunc("GET /v1/me", s.getMe)
	s.mux.HandleFunc("PUT /v1/me", s.putMe)
	return s
}

// Handler returns the fully wrapped handler.
func (s *Server) Handler() http.Handler {
	return logging(s.auth(s.mux))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if p, ok := s.bearer(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), personKey{}, p.ID)))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/media/") && s.signer.Verify(r.URL.Path, r.URL.Query()) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="sentinel"`)
		writeErr(w, http.StatusUnauthorized, "unauthorized")
	})
}

// bearer returns the person whose token the request carries. Every token is
// compared, in constant time, whether or not an earlier one matched.
func (s *Server) bearer(r *http.Request) (model.Person, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return model.Person{}, false
	}
	got := []byte(strings.TrimSpace(h[len(prefix):]))
	var who model.Person
	found := false
	for _, p := range s.people {
		if subtle.ConstantTimeCompare(got, []byte(p.Token)) == 1 {
			who, found = p, true
		}
	}
	return who, found
}

type personKey struct{}

// person is who is asking: set by auth for every bearer-authenticated
// request. It is empty only for signed media requests, which no handler
// below serves.
func person(r *http.Request) string {
	id, _ := r.Context().Value(personKey{}).(string)
	return id
}

// ---- me ---------------------------------------------------------------------

type meView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OutpostUserID string `json:"outpost_user_id"`
}

func (s *Server) me(r *http.Request) (meView, error) {
	id := person(r)
	v := meView{ID: id, Name: model.Person{ID: id}.Name()}
	user, _, err := s.store.GetPersonSetting(r.Context(), id, store.OutpostUserKey)
	v.OutpostUserID = user
	return v, err
}

// GET /v1/me: who this token belongs to, and their Outpost user id.
func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	v, err := s.me(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// PUT /v1/me {"outpost_user_id": "..."}
//
// The Outpost (dashboard) user that dashboard links open as for this person,
// in the app and in their pushes. Empty clears it, falling back to the
// server's DASHBOARD_LINK_USER_ID. It signs a browser in, so it is stored
// but never logged.
func (s *Server) putMe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutpostUserID *string `json:"outpost_user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if body.OutpostUserID != nil {
		user := strings.TrimSpace(*body.OutpostUserID)
		if len(user) > 128 || strings.ContainsAny(user, " \t\r\n&?#/") {
			writeErr(w, http.StatusBadRequest, "outpost_user_id is not a user id")
			return
		}
		if err := s.store.SetPersonSetting(r.Context(), person(r), store.OutpostUserKey, user); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.getMe(w, r)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if r.URL.Path == "/healthz" {
			return
		}
		// Query strings may carry media signatures; log the path only.
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---- health -----------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	st := s.engine.Stats()
	body := map[string]any{
		"ok":          s.engine.Healthy(),
		"uptime_sec":  int(time.Since(s.started).Seconds()),
		"mqtt":        st,
		"push":        s.pusher.Enabled(),
		"sse_clients": s.hub.Clients(),
		"dashboard":   s.dash.Status(),
	}
	code := http.StatusOK
	if !s.engine.Healthy() {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
}

// ---- alerts -----------------------------------------------------------------

// decorator returns the function that finishes an alert for one person on
// its way out: signed media links for camera alerts, and the dashboard link
// opened as that person's Outpost user.
func (s *Server) decorator(ctx context.Context, who string) func(model.Alert) model.Alert {
	user, _, _ := s.store.GetPersonSetting(ctx, who, store.OutpostUserKey)
	return func(a model.Alert) model.Alert {
		if a.Kind == model.KindCamera {
			a.Media = s.signer.MediaFor(a)
		}
		a.Link = dashboard.LinkFor(a.Link, user)
		return a
	}
}

// GET /v1/alerts?status=&kind=&source=&since=&cursor=&limit=&scope=
//
// scope is "mine" (the default: the asker's own history, without what was
// muted or filtered out for them when it fired) or "all" (everything that
// was recorded). When a full page comes back, next_cursor continues it.

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ListFilter{Status: q.Get("status"), Kind: q.Get("kind"), Source: q.Get("source")}
	switch q.Get("scope") {
	case "", "mine":
		f.VisibleTo = person(r)
	case "all":
	default:
		writeErr(w, http.StatusBadRequest, "scope must be mine or all")
		return
	}
	if v := q.Get("cursor"); v != "" {
		c, err := store.ParseCursor(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad cursor")
			return
		}
		f.Before = c
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since must be RFC3339")
			return
		}
		f.Since = t
	}
	if v := q.Get("limit"); v != "" {
		var n int
		if _, err := fmtSscan(v, &n); err != nil {
			writeErr(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		f.Limit = n
	}
	alerts, err := s.store.List(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	decorate := s.decorator(r.Context(), person(r))
	for i := range alerts {
		alerts[i] = decorate(alerts[i])
	}
	resp := map[string]any{"alerts": alerts, "server_time": time.Now().UTC()}
	if len(alerts) > 0 && len(alerts) == store.ListLimit(f.Limit) {
		resp["next_cursor"] = store.CursorAfter(alerts[len(alerts)-1]).String()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getAlert(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Get(r.Context(), r.PathValue("id"))
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "no such alert")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.decorator(r.Context(), person(r))(a))
}

func (s *Server) resolveAlert(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Resolve(r.Context(), r.PathValue("id"), time.Now())
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "no such alert")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(model.Event{Type: "updated", Alert: a})
	// Dashboard alerts: dismiss in the dashboard too, so the bell and the
	// phone agree. Best effort; the app's resolve already succeeded.
	if a.Kind == model.KindDashboard && a.Dashboard != nil && s.dash.MarkSeenEnabled() {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := s.dash.MarkSeen(ctx, id); err != nil {
				slog.Warn("dashboard: mark seen failed", "alert", id, "error", err)
			}
		}(a.Dashboard.AlertID)
	}
	writeJSON(w, http.StatusOK, s.decorator(r.Context(), person(r))(a))
}

// ---- devices ----------------------------------------------------------------

func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request) {
	var d model.Device
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	d.Token = strings.ToLower(strings.TrimSpace(d.Token))
	if len(d.Token) < 32 || !isHex(d.Token) {
		writeErr(w, http.StatusBadRequest, "token must be the hex APNs device token")
		return
	}
	switch d.Env {
	case model.EnvSandbox, model.EnvProduction:
	case "":
		d.Env = model.EnvSandbox
	default:
		writeErr(w, http.StatusBadRequest, "env must be sandbox or production")
		return
	}
	// The phone belongs to whoever registered it; registering again with
	// another person's token moves it.
	d.Person = person(r)
	if err := s.store.UpsertDevice(r.Context(), d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("device registered", "person", d.Person, "env", d.Env, "name", d.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	devs, err := s.store.ListDevices(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devs})
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteDevice(r.Context(), strings.ToLower(r.PathValue("token"))); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- push test --------------------------------------------------------------

func (s *Server) testPush(w http.ResponseWriter, r *http.Request) {
	if !s.pusher.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "push not configured (APNS_* unset)")
		return
	}
	var body struct {
		Token string `json:"token"`
		Env   string `json:"env"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if body.Env == "" {
		body.Env = model.EnvSandbox
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"results": s.pusher.SendTest(ctx, strings.ToLower(body.Token), body.Env, person(r))})
}

// ---- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
