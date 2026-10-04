package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/frigate"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/ingest"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/policy"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

const testToken = "0123456789abcdef"

const actionRules = `
rules:
  - name: garage
    topic: "zigbee2mqtt/Large Garage Door"
    condition: {field: contact, operator: eq, value: false}
    alert: {duration_minutes: 30, repeat_minutes: 60, severity: warning, message: "open"}
  - name: nightlight_hall
    topic: "zigbee2mqtt/night-light-hall"
    condition: {field: occupancy, operator: eq, value: true}
    alert: {duration_minutes: 0, severity: info, message: "Motion in the hall"}
    action:
      topic: "zigbee2mqtt/night-light-hall/set"
      payload: '{"state":"ON"}'
      active_topic: automation/night-light-hall/active
  - name: siren
    topic: "zigbee2mqtt/Freezer"
    condition: {field: temp, operator: gt, value: 0}
    alert: {severity: critical, message: "warm"}
    action: {topic: "notify/siren/set", payload: '{"state":"ON"}', active: false}
  - name: alert_switched_off
    topic: "zigbee2mqtt/Office"
    condition: {field: temp, operator: gt, value: 76}
    alert: {severity: warning, message: "warm", active: false}
  - name: light_only
    topic: "zigbee2mqtt/night-light-attic"
    condition: {field: occupancy, operator: eq, value: true}
    action:
      topic: "zigbee2mqtt/night-light-attic/set"
      payload: '{"state":"ON"}'
      active_topic: automation/night-light-attic/active
`

// actionServer is a server over a real store and rules file, with an MQTT
// engine that was never connected: enough for everything short of a publish.
func actionServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(actionRules), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no frigate here", http.StatusServiceUnavailable)
	}))
	t.Cleanup(fr.Close)
	reader := rules.NewReader(path, time.Minute)
	return New(Deps{
		Token: testToken, Store: st, Rules: reader,
		Engine:  ingest.New("tcp://127.0.0.1:1", "test", st, nil, reader),
		Decider: policy.NewDecider(st, time.UTC),
		Cameras: frigate.NewCameras(fr.URL, time.Minute), FrigateURL: fr.URL,
	}).Handler()
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSourcesReportTheRuleAction(t *testing.T) {
	rec := call(t, actionServer(t), http.MethodGet, "/v1/sources", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Sources []struct {
			ID     string `json:"id"`
			Action *struct {
				Active     bool `json:"active"`
				Switchable bool `json:"switchable"`
			} `json:"action"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	type action struct{ present, active, switchable bool }
	got := map[string]action{}
	for _, s := range resp.Sources {
		a := action{}
		if s.Action != nil {
			a = action{true, s.Action.Active, s.Action.Switchable}
		}
		got[s.ID] = a
	}
	want := map[string]action{
		"garage":          {},                 // alert only: no action block
		"nightlight_hall": {true, true, true}, // nothing retained: the default, active
		"siren":           {true, false, false},
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("%s: not listed", id)
		} else if g != w {
			t.Errorf("%s: got %+v want %+v", id, g, w)
		}
	}
	// An alert switched off in the config never fires; an action-only rule
	// never notifies. Neither is a source.
	for _, id := range []string{"alert_switched_off", "light_only"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s: listed as a source", id)
		}
	}
}

func TestPutActionRejections(t *testing.T) {
	h := actionServer(t)
	cases := []struct {
		name, id, body string
		want           int
	}{
		{"unknown source", "nope", `{"active": false}`, http.StatusNotFound},
		{"action-only rule is not a source", "light_only", `{"active": false}`, http.StatusNotFound},
		{"alert-only source", "garage", `{"active": false}`, http.StatusConflict},
		{"action without an active_topic", "siren", `{"active": true}`, http.StatusConflict},
		{"empty body", "nightlight_hall", ``, http.StatusBadRequest},
		{"missing key", "nightlight_hall", `{}`, http.StatusBadRequest},
		{"wrong type", "nightlight_hall", `{"active": "off"}`, http.StatusBadRequest},
		// A good request that cannot reach the broker must fail loudly, not
		// report a switch that Marshal never heard about.
		{"broker unreachable", "nightlight_hall", `{"active": false}`, http.StatusBadGateway},
	}
	for _, c := range cases {
		rec := call(t, h, http.MethodPut, "/v1/sources/"+c.id+"/action", c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status %d want %d (%s)", c.name, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
	}
	// The failed publish must not have been recorded as the new state.
	rec := call(t, h, http.MethodGet, "/v1/sources", "")
	if !strings.Contains(rec.Body.String(), `"action":{"active":true,"switchable":true}`) {
		t.Errorf("nightlight_hall should still be active: %s", rec.Body)
	}
}
