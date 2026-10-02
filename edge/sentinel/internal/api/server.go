// Package api is sentinel's HTTP surface: the JSON API the iOS app uses, an
// SSE stream for the foreground app, and a media proxy to Frigate.
//
// Auth is a single bearer token. Media paths additionally accept a signed
// URL (see media.go) because AVPlayer, AsyncImage and the notification
// extension cannot reliably send headers.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/ingest"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/push"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

type Server struct {
	token   string
	store   *store.Store
	rules   *rules.Reader
	pusher  *push.Pusher
	engine  *ingest.Engine
	signer  *Signer
	hub     *Hub
	proxy   http.Handler
	mux     *http.ServeMux
	started time.Time
}

type Deps struct {
	Token      string
	Store      *store.Store
	Rules      *rules.Reader
	Pusher     *push.Pusher
	Engine     *ingest.Engine
	Signer     *Signer
	Hub        *Hub
	FrigateURL string
}

func New(d Deps) *Server {
	s := &Server{token: d.Token, store: d.Store, rules: d.Rules, pusher: d.Pusher, engine: d.Engine,
		signer: d.Signer, hub: d.Hub, mux: http.NewServeMux(), started: time.Now()}
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
		if s.bearerOK(r) {
			next.ServeHTTP(w, r)
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

func (s *Server) bearerOK(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return false
	}
	got := strings.TrimSpace(h[len(p):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
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
	}
	code := http.StatusOK
	if !s.engine.Healthy() {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
}

// ---- alerts -----------------------------------------------------------------

func (s *Server) decorate(a model.Alert) model.Alert {
	if a.Kind == model.KindCamera {
		a.Media = s.signer.MediaFor(a)
	}
	return a
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ListFilter{Status: q.Get("status"), Kind: q.Get("kind")}
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
	for i := range alerts {
		alerts[i] = s.decorate(alerts[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts, "server_time": time.Now().UTC()})
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
	writeJSON(w, http.StatusOK, s.decorate(a))
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
	writeJSON(w, http.StatusOK, s.decorate(a))
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
	if err := s.store.UpsertDevice(r.Context(), d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("device registered", "env", d.Env, "name", d.Name)
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
	writeJSON(w, http.StatusOK, map[string]any{"results": s.pusher.SendTest(ctx, strings.ToLower(body.Token), body.Env)})
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
