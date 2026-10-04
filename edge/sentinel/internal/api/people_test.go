package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

type sourcesResp struct {
	Sources []struct {
		ID     string `json:"id"`
		Muted  bool   `json:"muted"`
		Policy struct {
			Passive      bool `json:"passive"`
			AlwaysNotify bool `json:"always_notify"`
		} `json:"policy"`
	} `json:"sources"`
	KindMutes map[string]struct {
		Muted bool `json:"muted"`
	} `json:"kind_mutes"`
	QuietHours struct {
		Enabled bool   `json:"enabled"`
		Start   string `json:"start"`
	} `json:"quiet_hours"`
}

func TestTheTokenSaysWhoIsAsking(t *testing.T) {
	h, _ := peopleServer(t)
	type me struct {
		ID, Name string
	}
	if got := decode[me](t, callAs(t, h, testToken, http.MethodGet, "/v1/me", "")); got.ID != "tom" || got.Name != "Tom" {
		t.Fatalf("tom's token: %+v", got)
	}
	if got := decode[me](t, callAs(t, h, mariaToken, http.MethodGet, "/v1/me", "")); got.ID != "maria" || got.Name != "Maria" {
		t.Fatalf("maria's token: %+v", got)
	}
	for _, bad := range []string{"", "nope-nope-nope-nope", testToken + "x", testToken[:15]} {
		if rec := callAs(t, h, bad, http.MethodGet, "/v1/me", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status %d", bad, rec.Code)
		}
	}
}

// What one person mutes, sets passive or schedules must never show up for,
// or silence, the other.
func TestSettingsBelongToThePersonWhoSetThem(t *testing.T) {
	h, _ := peopleServer(t)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/v1/sources/garage/mute", `{"minutes": null}`},
		{http.MethodPut, "/v1/sources/nightlight_hall/policy", `{"passive": true, "always_notify": true}`},
		{http.MethodPost, "/v1/sources/kind/camera/mute", `{"minutes": 60}`},
		{http.MethodPut, "/v1/settings/quiet-hours", `{"enabled": true, "start": "21:30"}`},
	} {
		if rec := callAs(t, h, mariaToken, c.method, c.path, c.body); rec.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	state := func(token string) (garageMuted, hallPassive, cameraKindMuted, quiet bool, start string) {
		r := decode[sourcesResp](t, callAs(t, h, token, http.MethodGet, "/v1/sources", ""))
		for _, s := range r.Sources {
			switch s.ID {
			case "garage":
				garageMuted = s.Muted
			case "nightlight_hall":
				hallPassive = s.Policy.Passive && s.Policy.AlwaysNotify
			}
		}
		return garageMuted, hallPassive, r.KindMutes["camera"].Muted, r.QuietHours.Enabled, r.QuietHours.Start
	}
	if g, p, k, q, start := state(mariaToken); !g || !p || !k || !q || start != "21:30" {
		t.Fatalf("maria should see her own settings: %v %v %v %v %s", g, p, k, q, start)
	}
	if g, p, k, q, start := state(testToken); g || p || k || q || start != "23:00" {
		t.Fatalf("maria's settings leaked to tom: %v %v %v %v %s", g, p, k, q, start)
	}
	// The legacy rule-mute alias is per person too.
	type ruleList struct {
		Rules []struct {
			Name  string `json:"name"`
			Muted bool   `json:"muted"`
		} `json:"rules"`
	}
	for token, want := range map[string]bool{mariaToken: true, testToken: false} {
		for _, r := range decode[ruleList](t, callAs(t, h, token, http.MethodGet, "/v1/rules", "")).Rules {
			if r.Name == "garage" && r.Muted != want {
				t.Errorf("/v1/rules garage muted=%v for token %s…", r.Muted, token[:4])
			}
		}
	}
}

func TestDevicesBelongToWhoeverRegisteredThem(t *testing.T) {
	h, st := peopleServer(t)
	tok := func(c string) string { return strings.Repeat(c, 64) }
	for token, dev := range map[string]string{testToken: tok("a"), mariaToken: tok("b")} {
		body := fmt.Sprintf(`{"token": %q, "env": "sandbox", "name": "phone"}`, dev)
		if rec := callAs(t, h, token, http.MethodPost, "/v1/devices", body); rec.Code != http.StatusOK {
			t.Fatalf("register: %d %s", rec.Code, rec.Body)
		}
	}
	devs, _ := st.ListDevices(context.Background())
	got := map[string]string{}
	for _, d := range devs {
		got[d.Token] = d.Person
	}
	if got[tok("a")] != "tom" || got[tok("b")] != "maria" {
		t.Fatalf("device owners: %v", got)
	}
	// Signing in on that phone as someone else moves it.
	callAs(t, h, mariaToken, http.MethodPost, "/v1/devices", fmt.Sprintf(`{"token": %q, "env": "sandbox"}`, tok("a")))
	devs, _ = st.ListDevices(context.Background())
	for _, d := range devs {
		if d.Token == tok("a") && d.Person != "maria" {
			t.Fatalf("re-registered phone still belongs to %s", d.Person)
		}
	}
}

type alertsResp struct {
	Alerts []struct {
		ID   string `json:"id"`
		Rule string `json:"rule"`
		Kind string `json:"kind"`
		Link string `json:"link"`
	} `json:"alerts"`
	NextCursor string `json:"next_cursor"`
}

func seed(t *testing.T, st *store.Store, rule string, at time.Time) model.Alert {
	t.Helper()
	a, _, err := st.UpsertSensor(context.Background(), store.SensorMsg{Rule: rule, Severity: "info", Message: rule, At: at}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Close it so the next one for the same rule is a new alert.
	if _, _, err := st.ResolveByRule(context.Background(), rule, at.Add(time.Second), ""); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestHistoryIsFilteredPagedAndPerPerson(t *testing.T) {
	h, st := peopleServer(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	var hall []model.Alert
	for i := 0; i < 5; i++ {
		hall = append(hall, seed(t, st, "nightlight_hall", base.Add(time.Duration(i)*time.Minute)))
	}
	garage := seed(t, st, "garage", base.Add(10*time.Minute))
	// Maria had the hall muted when two of those fired.
	for _, a := range hall[:2] {
		if err := st.SetHidden(ctx, a.ID, "maria", "source muted"); err != nil {
			t.Fatal(err)
		}
	}
	list := func(token, query string) alertsResp {
		return decode[alertsResp](t, callAs(t, h, token, http.MethodGet, "/v1/alerts?"+query, ""))
	}

	if n := len(list(testToken, "").Alerts); n != 6 {
		t.Fatalf("tom sees %d, want all 6", n)
	}
	if n := len(list(mariaToken, "").Alerts); n != 4 {
		t.Fatalf("maria sees %d, want 4 (two were muted for her)", n)
	}
	if n := len(list(mariaToken, "scope=all").Alerts); n != 6 {
		t.Fatalf("maria, everything: %d, want 6", n)
	}
	if rec := callAs(t, h, testToken, http.MethodGet, "/v1/alerts?scope=theirs", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad scope: %d", rec.Code)
	}

	// One source.
	got := list(testToken, "source=garage")
	if len(got.Alerts) != 1 || got.Alerts[0].ID != garage.ID {
		t.Fatalf("source filter: %+v", got.Alerts)
	}
	if n := len(list(mariaToken, "source=nightlight_hall").Alerts); n != 3 {
		t.Fatalf("maria, hall only: %d, want 3", n)
	}
	if n := len(list(testToken, "kind=camera").Alerts); n != 0 {
		t.Fatalf("kind filter: %d", n)
	}

	// Paging walks the whole set once, newest first, with no cursor at the end.
	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		q := "limit=2&source=nightlight_hall"
		if cursor != "" {
			q += "&cursor=" + strings.ReplaceAll(cursor, "+", "%2B")
		}
		r := list(testToken, q)
		for _, a := range r.Alerts {
			seen = append(seen, a.ID)
		}
		if cursor = r.NextCursor; cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("paged %d alerts, want 5: %v", len(seen), seen)
	}
	for i, id := range seen {
		if want := hall[len(hall)-1-i].ID; id != want {
			t.Fatalf("page order at %d: got %s want %s", i, id, want)
		}
	}
	if rec := callAs(t, h, testToken, http.MethodGet, "/v1/alerts?cursor=garbage", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", rec.Code)
	}
	// A hidden alert is still reachable by id: a tapped notification must
	// never 404, whoever taps it.
	if rec := callAs(t, h, mariaToken, http.MethodGet, "/v1/alerts/"+hall[0].ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("hidden alert by id: %d", rec.Code)
	}
}

func TestOutpostUserIsPerPersonAndSignsTheirLinks(t *testing.T) {
	h, st := peopleServer(t)
	ctx := context.Background()
	// Stored the way the dashboard feed stores it: the server default baked in.
	a, _, err := st.UpsertDashboard(ctx, store.DashboardMsg{ExternalID: "m1", SourceID: "dashboard_high-temp", RuleName: "High Temp",
		Title: "High Temp", Severity: "warning", DashboardID: "d1",
		Link: "http://dash.example/view/dashboards/d1?user_id=server-default&var_host=a"})
	if err != nil {
		t.Fatal(err)
	}
	type me struct {
		OutpostUserID string `json:"outpost_user_id"`
	}
	link := func(token string) string {
		for _, x := range decode[alertsResp](t, callAs(t, h, token, http.MethodGet, "/v1/alerts", "")).Alerts {
			if x.ID == a.ID {
				return x.Link
			}
		}
		t.Fatal("alert not listed")
		return ""
	}

	// Nobody has set one: the stored link is used as it is.
	if got := link(mariaToken); !strings.Contains(got, "user_id=server-default") {
		t.Fatalf("default link: %s", got)
	}
	if got := decode[me](t, callAs(t, h, mariaToken, http.MethodPut, "/v1/me", `{"outpost_user_id": " maria-guid "}`)); got.OutpostUserID != "maria-guid" {
		t.Fatalf("put me: %+v", got)
	}
	if got := link(mariaToken); !strings.Contains(got, "user_id=maria-guid") || strings.Contains(got, "server-default") || !strings.Contains(got, "var_host=a") {
		t.Fatalf("maria's link: %s", got)
	}
	if got := link(testToken); !strings.Contains(got, "user_id=server-default") {
		t.Fatalf("tom's link changed: %s", got)
	}
	// By id too, which is what a tapped push fetches.
	one := decode[struct {
		Link string `json:"link"`
	}](t, callAs(t, h, mariaToken, http.MethodGet, "/v1/alerts/"+a.ID, ""))
	if !strings.Contains(one.Link, "user_id=maria-guid") {
		t.Fatalf("by id: %s", one.Link)
	}
	if got := decode[me](t, callAs(t, h, testToken, http.MethodGet, "/v1/me", "")); got.OutpostUserID != "" {
		t.Fatalf("tom has maria's id: %+v", got)
	}
	// Not a user id.
	for _, bad := range []string{`{"outpost_user_id": "a b"}`, `{"outpost_user_id": "x&admin=1"}`, `{"outpost_user_id": "` + strings.Repeat("x", 200) + `"}`} {
		if rec := callAs(t, h, mariaToken, http.MethodPut, "/v1/me", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", bad[:30], rec.Code)
		}
	}
	// Clearing goes back to the default.
	callAs(t, h, mariaToken, http.MethodPut, "/v1/me", `{"outpost_user_id": ""}`)
	if got := link(mariaToken); !strings.Contains(got, "user_id=server-default") {
		t.Fatalf("after clearing: %s", got)
	}
}

// The history filter: several groups and sources at once, as a union, and a
// time range an alert was open in.
func TestHistorySelectsSeveralSourcesAndATimeRange(t *testing.T) {
	h, st := peopleServer(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Hour).Truncate(time.Second)
	at := func(hours float64) time.Time { return base.Add(time.Duration(hours * float64(time.Hour))) }
	stamp := func(tm time.Time) string { return tm.UTC().Format(time.RFC3339) }

	hall := []model.Alert{seed(t, st, "nightlight_hall", at(0)), seed(t, st, "nightlight_hall", at(2)), seed(t, st, "nightlight_hall", at(4))}
	garage := seed(t, st, "garage", at(1))
	// Still open: started at +3h and never heard from again.
	siren, _, err := st.UpsertSensor(ctx, store.SensorMsg{Rule: "siren", Severity: "critical", Message: "warm", At: at(3)}, false)
	if err != nil {
		t.Fatal(err)
	}
	dash, _, err := st.UpsertDashboard(ctx, store.DashboardMsg{ExternalID: "m1", SourceID: "dashboard_high-temp", RuleName: "High Temp",
		Title: "High Temp", Severity: "warning", FiredAt: at(9)})
	if err != nil {
		t.Fatal(err)
	}

	ids := func(query string) map[string]bool {
		out := map[string]bool{}
		for _, a := range decode[alertsResp](t, call(t, h, http.MethodGet, "/v1/alerts?"+query, "")).Alerts {
			out[a.ID] = true
		}
		return out
	}
	want := func(name, query string, alerts ...model.Alert) {
		t.Helper()
		got := ids(query)
		if len(got) != len(alerts) {
			t.Errorf("%s: %d alerts, want %d (%s)", name, len(got), len(alerts), query)
			return
		}
		for _, a := range alerts {
			if !got[a.ID] {
				t.Errorf("%s: missing %s %s", name, a.Rule, a.ID)
			}
		}
	}
	all := append(append([]model.Alert{}, hall...), garage, siren, dash)

	want("no filter", "", all...)
	want("two sources, comma", "source=garage,nightlight_hall", append(append([]model.Alert{}, hall...), garage)...)
	want("two sources, repeated", "source=garage&source=nightlight_hall", append(append([]model.Alert{}, hall...), garage)...)
	want("two groups", "kind=sensor,dashboard", all...)
	want("one group", "kind=dashboard", dash)
	// A whole group plus one source from another: the union, not the overlap.
	want("group plus a source", "kind=dashboard&source=garage", dash, garage)
	want("group that has nothing plus a source", "kind=camera&source=siren", siren)

	// Time range. Each seeded alert is open for one second from its start.
	want("a window around +2h", "from="+stamp(at(1.5))+"&to="+stamp(at(2.5)), hall[1])
	// The siren started at +3h and is still open, so it is in every window
	// from then on, even though nothing has been heard from it since.
	want("a window after everything closed", "from="+stamp(at(5))+"&to="+stamp(at(6)), siren)
	want("up to +1h", "to="+stamp(at(1)), hall[0], garage)
	want("since +3.5h", "from="+stamp(at(3.5)), hall[2], siren, dash)
	// The range and the selection narrow each other.
	want("hall only, since +1.5h", "source=nightlight_hall&from="+stamp(at(1.5)), hall[1], hall[2])
	want("window with a group plus a source", "kind=dashboard&source=garage&to="+stamp(at(8)), garage)

	for name, q := range map[string]string{
		"to before from": "from=" + stamp(at(5)) + "&to=" + stamp(at(4)),
		"bad from":       "from=yesterday",
		"bad to":         "to=2026-10-03",
		"too many":       "source=" + strings.Repeat("x,", 250),
	} {
		if rec := call(t, h, http.MethodGet, "/v1/alerts?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	// Paging holds under a selection and a range together.
	var seen int
	cursor := ""
	for page := 0; page < 10; page++ {
		q := "limit=2&kind=dashboard&source=nightlight_hall,garage&from=" + stamp(at(-1))
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		r := decode[alertsResp](t, call(t, h, http.MethodGet, "/v1/alerts?"+q, ""))
		seen += len(r.Alerts)
		if cursor = r.NextCursor; cursor == "" {
			break
		}
	}
	if seen != 5 {
		t.Errorf("paged %d under a filter, want 5", seen)
	}
}
