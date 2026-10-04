package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/policy"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

// source is one thing that can notify the phone: a Marshal alert rule or a
// Frigate camera. Its id equals the alert's `rule` field, which is what the
// mutes and policies tables are keyed by.
type source struct {
	ID            string       `json:"id"`
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	Rule          string       `json:"rule,omitempty"`
	Camera        string       `json:"camera,omitempty"`
	Severity      string       `json:"severity,omitempty"`
	RepeatMinutes int          `json:"repeat_minutes,omitempty"`
	Muted         bool         `json:"muted"`
	MutedUntil    *time.Time   `json:"muted_until"`
	Policy        store.Policy `json:"policy"`
	// Action is set for a sensor source whose rule also acts (the
	// nightlights: motion both notifies and switches the light).
	Action *sourceAction `json:"action,omitempty"`
}

// sourceAction is the Marshal action on a source's rule (0.5.0). Active is
// whether it may start new cycles; Switchable is false for a rule with no
// active_topic, where only the config can change it. Owner is Marshal's
// state_topic value (automation | override | parked), reported for context:
// a parked or overridden action does nothing whatever Active says.
type sourceAction struct {
	Active     bool   `json:"active"`
	Switchable bool   `json:"switchable"`
	Owner      string `json:"owner,omitempty"`
}

func (s *Server) sourceAction(r rules.Rule) *sourceAction {
	if !r.HasAction {
		return nil
	}
	a := &sourceAction{Active: s.actionActive(r), Switchable: r.ActiveTopic != ""}
	if r.StateTopic != "" {
		a.Owner = s.engine.Retained(r.StateTopic)
	}
	return a
}

// kindMute is the whole-kind mute state reported under "kind_mutes".
type kindMute struct {
	Muted      bool       `json:"muted"`
	MutedUntil *time.Time `json:"muted_until"`
}

var muteableKinds = []string{model.KindSensor, model.KindDashboard, model.KindCamera}

func (s *Server) kindMutes(mutes map[string]store.Mute) map[string]kindMute {
	out := map[string]kindMute{}
	for _, k := range muteableKinds {
		km := kindMute{}
		if m, ok := mutes[policy.KindMuteKey(k)]; ok {
			km.Muted, km.MutedUntil = true, m.Until
		}
		out[k] = km
	}
	return out
}

type quietView struct {
	policy.QuietHours
	Timezone  string `json:"timezone"`
	ActiveNow bool   `json:"active_now"`
}

func (s *Server) quietView(r *http.Request) quietView {
	q, _ := s.decider.QuietHours(r.Context())
	return quietView{QuietHours: q, Timezone: s.decider.Location().String(), ActiveNow: q.ActiveAt(time.Now(), s.decider.Location())}
}

// sources builds the list: alert rules from rules.yaml, cameras from
// Frigate's config (falling back to cameras seen in stored alerts).
func (s *Server) sources(r *http.Request) ([]source, error) {
	ctx := r.Context()
	rs, _ := s.rules.Rules()
	mutes, err := s.store.Mutes(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	pols, err := s.store.Policies(ctx)
	if err != nil {
		return nil, err
	}
	var out []source
	for _, rule := range rs {
		if !rule.HasAlert {
			continue
		}
		out = append(out, source{ID: rule.Name, Kind: model.KindSensor, Name: humanize(rule.Name), Rule: rule.Name,
			Severity: rule.Severity, RepeatMinutes: rule.RepeatMinutes, Action: s.sourceAction(rule)})
	}
	cams, _ := s.cameras.Names(ctx)
	seen := map[string]bool{}
	for _, c := range cams {
		seen[c] = true
		out = append(out, source{ID: "frigate_" + c, Kind: model.KindCamera, Name: humanize(c), Camera: c})
	}
	// dashboard (ts-store) rules that have fired: id dashboard_<slug>, name = rule
	if names, err := s.store.RecentSourceNames(ctx, model.KindDashboard, 100); err == nil {
		for _, n := range names {
			out = append(out, source{ID: n[0], Kind: model.KindDashboard, Name: n[1], Rule: n[1]})
		}
	}
	// cameras that alerted but are not (or no longer) in Frigate's config
	if recent, err := s.store.List(ctx, store.ListFilter{Kind: model.KindCamera, Limit: 200}); err == nil {
		for _, a := range recent {
			if a.Camera != "" && !seen[a.Camera] {
				seen[a.Camera] = true
				out = append(out, source{ID: "frigate_" + a.Camera, Kind: model.KindCamera, Name: humanize(a.Camera), Camera: a.Camera})
			}
		}
	}
	for i := range out {
		if m, ok := mutes[out[i].ID]; ok {
			out[i].Muted, out[i].MutedUntil = true, m.Until
		}
		if p, ok := pols[out[i].ID]; ok {
			out[i].Policy = p
		} else {
			out[i].Policy = store.DefaultPolicy()
		}
	}
	rank := map[string]int{model.KindSensor: 0, model.KindDashboard: 1, model.KindCamera: 2}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return rank[out[i].Kind] < rank[out[j].Kind]
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *Server) findSource(r *http.Request, id string) (source, bool, error) {
	all, err := s.sources(r)
	if err != nil {
		return source{}, false, err
	}
	for _, src := range all {
		if src.ID == id {
			return src, true, nil
		}
	}
	return source{}, false, nil
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	all, err := s.sources(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if all == nil {
		all = []source{}
	}
	mutes, err := s.store.Mutes(r.Context(), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": all, "kind_mutes": s.kindMutes(mutes), "quiet_hours": s.quietView(r)})
}

// parseMuteBody reads {"minutes": n|null} or {"until": RFC3339}. nil until
// means indefinite. Writes the 400 itself and returns ok=false on bad input.
func parseMuteBody(w http.ResponseWriter, r *http.Request) (until *time.Time, ok bool) {
	var body struct {
		Minutes *int    `json:"minutes"`
		Until   *string `json:"until"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body)
	switch {
	case body.Until != nil:
		t, err := time.Parse(time.RFC3339, *body.Until)
		if err != nil || !t.After(time.Now()) {
			writeErr(w, http.StatusBadRequest, "until must be a future RFC3339 time")
			return nil, false
		}
		return &t, true
	case body.Minutes != nil:
		if *body.Minutes <= 0 {
			writeErr(w, http.StatusBadRequest, "minutes must be > 0 or null")
			return nil, false
		}
		t := time.Now().Add(time.Duration(*body.Minutes) * time.Minute)
		return &t, true
	}
	return nil, true
}

// POST /v1/sources/kind/{kind}/mute — mute every source of a kind, including
// ones that have not fired yet (dashboard rules only appear after they do).
func (s *Server) muteKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !slices.Contains(muteableKinds, kind) {
		writeErr(w, http.StatusNotFound, "no such kind")
		return
	}
	until, ok := parseMuteBody(w, r)
	if !ok {
		return
	}
	if err := s.store.SetMute(r.Context(), policy.KindMuteKey(kind), until); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondKind(w, r, kind)
}

func (s *Server) unmuteKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !slices.Contains(muteableKinds, kind) {
		writeErr(w, http.StatusNotFound, "no such kind")
		return
	}
	if err := s.store.ClearMute(r.Context(), policy.KindMuteKey(kind)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondKind(w, r, kind)
}

func (s *Server) respondKind(w http.ResponseWriter, r *http.Request, kind string) {
	mutes, err := s.store.Mutes(r.Context(), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	km := s.kindMutes(mutes)[kind]
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "muted": km.Muted, "muted_until": km.MutedUntil})
}

// POST /v1/sources/{id}/mute {"minutes": n|null} or {"until": RFC3339}
func (s *Server) muteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok, err := s.findSource(r, id); err != nil || !ok {
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
		} else {
			writeErr(w, http.StatusNotFound, "no such source")
		}
		return
	}
	until, ok := parseMuteBody(w, r)
	if !ok {
		return
	}
	if err := s.store.SetMute(r.Context(), id, until); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondSource(w, r, id)
}

func (s *Server) unmuteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.ClearMute(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondSource(w, r, id)
}

// PUT /v1/sources/{id}/policy  (partial update)
func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok, err := s.findSource(r, id); err != nil || !ok {
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
		} else {
			writeErr(w, http.StatusNotFound, "no such source")
		}
		return
	}
	var body struct {
		Repeats      *string   `json:"repeats"`
		Objects      *[]string `json:"objects"`
		AlwaysNotify *bool     `json:"always_notify"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	pol, _ := s.store.GetPolicy(r.Context(), id)
	if body.Repeats != nil {
		switch *body.Repeats {
		case "none", "critical", "all":
			pol.Repeats = *body.Repeats
		default:
			writeErr(w, http.StatusBadRequest, "repeats must be none, critical or all")
			return
		}
	}
	if body.Objects != nil {
		objs := []string{}
		for _, o := range *body.Objects {
			if o = strings.ToLower(strings.TrimSpace(o)); o != "" {
				objs = append(objs, o)
			}
		}
		pol.Objects = objs
	}
	if body.AlwaysNotify != nil {
		pol.AlwaysNotify = *body.AlwaysNotify
	}
	if err := s.store.SetPolicy(r.Context(), id, pol); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondSource(w, r, id)
}

// PUT /v1/sources/{id}/action {"active": bool}
//
// Publishes to the rule's Marshal active_topic: "notify me, but leave the
// light alone". Retained either way, unlike enable: Marshal treats the value
// as state (a repeat of what it already holds is a no-op), and the broker
// replaying it is how the setting survives a Marshal restart. It is not
// parking: a cycle already running finishes and a manual override is kept.
func (s *Server) putAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule, ok := s.rules.Find(id)
	if !ok || !rule.HasAlert {
		writeErr(w, http.StatusNotFound, "no such source")
		return
	}
	if rule.ActiveTopic == "" {
		writeErr(w, http.StatusConflict, "rule has no active_topic; its action cannot be switched at runtime")
		return
	}
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil || body.Active == nil {
		writeErr(w, http.StatusBadRequest, `body must be {"active": true|false}`)
		return
	}
	payload := "false"
	if *body.Active {
		payload = "true"
	}
	if err := s.engine.PublishRetained(rule.ActiveTopic, payload); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("publish to %s: %v", rule.ActiveTopic, err))
		return
	}
	s.respondSource(w, r, id)
}

func (s *Server) respondSource(w http.ResponseWriter, r *http.Request, id string) {
	src, ok, err := s.findSource(r, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "no such source")
		return
	}
	writeJSON(w, http.StatusOK, src)
}

func (s *Server) getQuietHours(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.quietView(r))
}

// PUT /v1/settings/quiet-hours (partial update)
func (s *Server) putQuietHours(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool   `json:"enabled"`
		Start   *string `json:"start"`
		End     *string `json:"end"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	q, _ := s.decider.QuietHours(r.Context())
	if body.Enabled != nil {
		q.Enabled = *body.Enabled
	}
	if body.Start != nil {
		q.Start = *body.Start
	}
	if body.End != nil {
		q.End = *body.End
	}
	if err := s.decider.SetQuietHours(r.Context(), q); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.quietView(r))
}

func humanize(id string) string {
	w := strings.ReplaceAll(id, "_", " ")
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}
