// Package notify turns one alert event into what each person gets: a push,
// a passive push, nothing, and whether the alert belongs in their history.
// The decision is the policy package's; delivery is the push package's.
package notify

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/dashboard"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/policy"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/push"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

// Sender delivers a push to one person's phones (push.Pusher).
type Sender interface {
	SendAlert(ctx context.Context, a model.Alert, person string, passive bool) []push.Result
}

type Notifier struct {
	people  []model.Person
	decider *policy.Decider
	store   *store.Store
	sender  Sender // nil when push is not configured
}

// New builds a Notifier. sender may be nil: history is still recorded.
func New(people []model.Person, decider *policy.Decider, st *store.Store, sender Sender) *Notifier {
	return &Notifier{people: people, decider: decider, store: st, sender: sender}
}

// OnEvent decides and delivers for every person in turn. It runs before the
// event is streamed, so by the time a phone hears about an alert the server
// already knows whether that person should see it.
func (n *Notifier) OnEvent(ctx context.Context, ev model.Event) {
	for _, p := range n.people {
		d := n.decider.Decide(ctx, p.ID, ev)
		n.record(ctx, p.ID, ev, d)
		a := ev.Alert
		if !d.Push {
			if ev.Created || a.RepeatCount > 0 {
				slog.Info("push suppressed", "alert", a.ID, "person", p.ID, "source", a.Rule, "reason", d.Reason)
			}
			continue
		}
		if n.sender == nil {
			continue
		}
		if user, _, err := n.store.GetPersonSetting(ctx, p.ID, store.OutpostUserKey); err == nil {
			a.Link = dashboard.LinkFor(a.Link, user)
		}
		level := push.Level(a, d.Passive)
		for _, r := range n.sender.SendAlert(ctx, a, p.ID, d.Passive) {
			if r.Status == http.StatusOK {
				slog.Info("push sent", "alert", a.ID, "person", p.ID, "source", a.Rule, "level", level, "env", r.Env, "apns_id", r.ApnsID)
			} else {
				slog.Warn("push failed", "alert", a.ID, "person", p.ID, "env", r.Env, "status", r.Status, "reason", r.Reason)
			}
		}
	}
}

// record keeps a person's history in step with the decision. An alert is
// hidden from them when its first sighting was something they asked not to
// hear about; if a later event does push to them (the mute ran out and a
// repeat arrived), it reached them after all and is theirs again. Nothing
// else changes it: a mute set afterwards does not reach back into history.
func (n *Notifier) record(ctx context.Context, person string, ev model.Event, d policy.Decision) {
	var err error
	switch {
	case d.Push && !ev.Created:
		err = n.store.ClearHidden(ctx, ev.Alert.ID, person)
	case ev.Created && d.Hidden:
		err = n.store.SetHidden(ctx, ev.Alert.ID, person, d.Reason)
	}
	if err != nil {
		slog.Warn("history not recorded", "alert", ev.Alert.ID, "person", person, "error", err)
	}
}
