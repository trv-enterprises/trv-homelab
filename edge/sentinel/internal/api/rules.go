package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
)

// ruleView is a Marshal rule plus sentinel's two controls. "muted" is a
// sentinel-side push suppression (any rule); "owner" is Marshal's retained
// state_topic value for action rules (automation | override | parked),
// which is what enable/disable changes. They are different things and the
// app shows them separately. "active" (action rules only) is a third: whether
// the action may start new cycles, see actionActive.
type ruleView struct {
	rules.Rule
	Muted      bool       `json:"muted"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
	Owner      string     `json:"owner,omitempty"`
	Active     *bool      `json:"active,omitempty"`
}

func (s *Server) view(r rules.Rule, mutes map[string]storeMute) ruleView {
	v := ruleView{Rule: r}
	if m, ok := mutes[r.Name]; ok {
		v.Muted, v.MutedUntil = true, m.Until
	}
	if r.StateTopic != "" {
		v.Owner = s.engine.Retained(r.StateTopic)
	}
	if r.HasAction {
		active := s.actionActive(r)
		v.Active = &active
	}
	return v
}

// actionActive reports whether Marshal will start new cycles for the rule's
// action, resolved the way Marshal resolves it: the retained value on the
// rule's active_topic, else the configured default. Marshal does not report
// this anywhere (state_topic is ownership only), so the retained value both
// services read is the only truth there is.
func (s *Server) actionActive(r rules.Rule) bool {
	if r.ActiveTopic != "" {
		if active, ok := rules.ParseActive(s.engine.Retained(r.ActiveTopic)); ok {
			return active
		}
	}
	return r.ActiveDefault
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rs, err := s.rules.Rules()
	mutes, merr := s.mutes(r)
	if merr != nil {
		writeErr(w, http.StatusInternalServerError, merr.Error())
		return
	}
	out := make([]ruleView, 0, len(rs))
	for _, rule := range rs {
		out = append(out, s.view(rule, mutes))
	}
	resp := map[string]any{"rules": out}
	if err != nil {
		resp["warning"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /v1/rules/{name}/mute {"minutes": n|null}
func (s *Server) muteRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := s.rules.Find(r.PathValue("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such rule")
		return
	}
	var body struct {
		Minutes *int `json:"minutes"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body)
	var until *time.Time
	if body.Minutes != nil {
		if *body.Minutes <= 0 {
			writeErr(w, http.StatusBadRequest, "minutes must be > 0 or null")
			return
		}
		t := time.Now().Add(time.Duration(*body.Minutes) * time.Minute)
		until = &t
	}
	if err := s.store.SetMute(r.Context(), rule.Name, until); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondRule(w, r, rule)
}

// DELETE /v1/rules/{name}/mute
func (s *Server) unmuteRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := s.rules.Find(r.PathValue("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such rule")
		return
	}
	if err := s.store.ClearMute(r.Context(), rule.Name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondRule(w, r, rule)
}

// POST /v1/rules/{name}/enable {"enabled": bool}
//
// Publishes to the rule's Marshal enable_topic. Rules that share a topic are
// parked/resumed together (Marshal's design; the app groups them). "false"
// is published retained so a Marshal restart comes back parked; "true"
// clears the retained value instead of leaving a retained "true", because
// Marshal re-applies retained enables on every reconnect and a retained
// "true" would wipe any manual override each time.
func (s *Server) enableRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := s.rules.Find(r.PathValue("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such rule")
		return
	}
	if rule.EnableTopic == "" {
		writeErr(w, http.StatusConflict, "rule has no enable_topic; Marshal cannot park alert-only rules at runtime")
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil || body.Enabled == nil {
		writeErr(w, http.StatusBadRequest, `body must be {"enabled": true|false}`)
		return
	}
	var err error
	if *body.Enabled {
		if err = s.engine.Publish(rule.EnableTopic, 1, false, "true"); err == nil {
			err = s.engine.Publish(rule.EnableTopic, 1, true, "") // clear retained park
		}
	} else {
		err = s.engine.Publish(rule.EnableTopic, 1, true, "false")
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("publish to %s: %v", rule.EnableTopic, err))
		return
	}
	// Marshal publishes the new owner to state_topic almost immediately;
	// give it a beat so the response already reflects it.
	time.Sleep(300 * time.Millisecond)
	s.respondRule(w, r, rule)
}

func (s *Server) respondRule(w http.ResponseWriter, r *http.Request, rule rules.Rule) {
	mutes, err := s.mutes(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.view(rule, mutes))
}

type storeMute struct{ Until *time.Time }

func (s *Server) mutes(r *http.Request) (map[string]storeMute, error) {
	m, err := s.store.Mutes(r.Context(), time.Now())
	if err != nil {
		return nil, err
	}
	out := make(map[string]storeMute, len(m))
	for k, v := range m {
		out[k] = storeMute{Until: v.Until}
	}
	return out, nil
}
