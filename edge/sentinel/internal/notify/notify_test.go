package notify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/policy"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/push"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

type sent struct {
	alert   string
	person  string
	passive bool
	link    string
}

type fakeSender struct{ sent []sent }

func (f *fakeSender) SendAlert(_ context.Context, a model.Alert, person string, passive bool) []push.Result {
	f.sent = append(f.sent, sent{a.ID, person, passive, a.Link})
	return []push.Result{{Status: 200, Env: "sandbox"}}
}

// to lists who was pushed to since the last call, as "tom" or "maria(passive)".
func (f *fakeSender) to() string {
	var out []string
	for _, s := range f.sent {
		who := s.person
		if s.passive {
			who += "(passive)"
		}
		out = append(out, who)
	}
	f.sent = nil
	return strings.Join(out, ",")
}

var people = []model.Person{{ID: "tom"}, {ID: "maria"}}

func setup(t *testing.T) (*Notifier, *store.Store, *policy.Decider, *fakeSender) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	d := policy.NewDecider(st, time.UTC)
	f := &fakeSender{}
	return New(people, d, st, f), st, d, f
}

func created(id, rule string) model.Event {
	return model.Event{Type: "created", Created: true, Alert: model.Alert{ID: id, Kind: model.KindSensor, Rule: rule, Severity: "info", Status: model.StatusActive}}
}

func hidden(t *testing.T, st *store.Store, alert, person string) bool {
	t.Helper()
	h, err := st.IsHidden(context.Background(), alert, person)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestEachPersonGetsTheirOwnDecision(t *testing.T) {
	n, st, d, f := setup(t)
	ctx := context.Background()

	n.OnEvent(ctx, created("a1", "hall"))
	if got := f.to(); got != "tom,maria" {
		t.Fatalf("nobody has set anything: pushed to %q", got)
	}

	// Maria mutes the hall; Tom wants it, but passively.
	_ = st.SetMute(ctx, "maria", "hall", nil)
	_ = st.SetPolicy(ctx, "tom", "hall", store.Policy{Repeats: "critical", Passive: true})
	n.OnEvent(ctx, created("a2", "hall"))
	if got := f.to(); got != "tom(passive)" {
		t.Fatalf("pushed to %q", got)
	}
	if !hidden(t, st, "a2", "maria") || hidden(t, st, "a2", "tom") {
		t.Fatal("a2 should be out of maria's history and in tom's")
	}
	// What fired before she muted stays in her history.
	if hidden(t, st, "a1", "maria") {
		t.Fatal("a mute reached back into history")
	}
	// Another source is untouched for both.
	n.OnEvent(ctx, created("a3", "garage"))
	if got := f.to(); got != "tom,maria" {
		t.Fatalf("garage pushed to %q", got)
	}

	// Quiet hours hold the push but not the history: it is there in the
	// morning.
	_ = d.SetQuietHours(ctx, "tom", policy.QuietHours{Enabled: true, Start: "00:00", End: "23:59"})
	n.OnEvent(ctx, created("a4", "garage"))
	if got := f.to(); got != "maria" {
		t.Fatalf("tom is in quiet hours: pushed to %q", got)
	}
	if hidden(t, st, "a4", "tom") {
		t.Fatal("quiet hours hid the alert from tom's history")
	}
}

// An alert that was muted when it fired but later reaches the person (the
// mute ran out and a repeat arrived) is part of their history after all.
func TestALaterPushUnhides(t *testing.T) {
	n, st, _, f := setup(t)
	ctx := context.Background()
	_ = st.SetPolicy(ctx, "maria", "garage", store.Policy{Repeats: "all"})
	_ = st.SetMute(ctx, "maria", "garage", nil)
	n.OnEvent(ctx, created("g1", "garage"))
	f.to()
	if !hidden(t, st, "g1", "maria") {
		t.Fatal("muted first sighting should be hidden")
	}
	repeat := model.Event{Type: "updated", Alert: model.Alert{ID: "g1", Kind: model.KindSensor, Rule: "garage", Severity: "warning", Status: model.StatusActive, RepeatCount: 1}}
	// Still muted: the repeat does not push and changes nothing.
	n.OnEvent(ctx, repeat)
	if got := f.to(); got != "" || !hidden(t, st, "g1", "maria") {
		t.Fatalf("repeat while muted: pushed to %q", got)
	}
	_ = st.ClearMute(ctx, "maria", "garage")
	n.OnEvent(ctx, repeat)
	if got := f.to(); got != "maria" {
		t.Fatalf("repeat after unmute pushed to %q", got)
	}
	if hidden(t, st, "g1", "maria") {
		t.Fatal("it reached her, so it belongs in her history")
	}
	// A resolve is not a push and must not hide or unhide anything.
	_ = st.SetMute(ctx, "maria", "garage", nil)
	n.OnEvent(ctx, model.Event{Type: "updated", Alert: model.Alert{ID: "g1", Kind: model.KindSensor, Rule: "garage", Status: model.StatusResolved}})
	if hidden(t, st, "g1", "maria") {
		t.Fatal("a resolve while muted hid an alert she had already seen")
	}
}

func TestPushLinksOpenAsThePersonsOutpostUser(t *testing.T) {
	n, st, _, f := setup(t)
	ctx := context.Background()
	_ = st.SetPersonSetting(ctx, "maria", store.OutpostUserKey, "maria-guid")
	ev := created("d1", "dashboard_high-temp")
	ev.Alert.Kind = model.KindDashboard
	ev.Alert.Link = "http://dash.example/view/dashboards/x?user_id=server-default"
	n.OnEvent(ctx, ev)
	links := map[string]string{}
	for _, s := range f.sent {
		links[s.person] = s.link
	}
	if !strings.Contains(links["maria"], "user_id=maria-guid") || strings.Contains(links["maria"], "server-default") {
		t.Fatalf("maria's push link: %s", links["maria"])
	}
	if !strings.Contains(links["tom"], "user_id=server-default") {
		t.Fatalf("tom's push link: %s", links["tom"])
	}
}

// With push not configured the history is still kept.
func TestHistoryIsRecordedWithoutASender(t *testing.T) {
	_, st, d, _ := setup(t)
	ctx := context.Background()
	n := New(people, d, st, nil)
	_ = st.SetMute(ctx, "maria", "hall", nil)
	n.OnEvent(ctx, created("a1", "hall"))
	if !hidden(t, st, "a1", "maria") || hidden(t, st, "a1", "tom") {
		t.Fatal("history not recorded without push")
	}
}
