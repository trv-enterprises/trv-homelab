// Package push delivers alerts to registered iPhones through APNs.
//
// Two clients exist because the environment is a property of the DEVICE
// TOKEN, not the server: an Xcode-installed build carries
// aps-environment=development and its token is only valid against the
// sandbox gateway, while a distribution-signed build of the same app gets a
// production token. The app reports which it has when it registers.
package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

type Config struct {
	Key      []byte // .p8 PEM
	KeyID    string
	TeamID   string
	BundleID string
}

// DeviceStore is the slice of the store the pusher needs.
type DeviceStore interface {
	ListDevices(ctx context.Context) ([]model.Device, error)
	DeleteDevice(ctx context.Context, token string) error
}

// LinkSigner mints the signed thumbnail URL embedded in the push so the
// notification extension can attach an image without any credential.
type LinkSigner interface {
	ThumbURL(a model.Alert) string
}

type Pusher struct {
	sandbox, production *apns2.Client
	topic               string
	devices             DeviceStore
	links               LinkSigner
}

// New builds a Pusher. A nil return with nil error means push is disabled
// (no APNs config); callers should treat that as a no-op sender.
func New(cfg Config, devices DeviceStore, links LinkSigner) (*Pusher, error) {
	if len(cfg.Key) == 0 {
		return nil, nil
	}
	authKey, err := token.AuthKeyFromBytes(cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("apns key: %w", err)
	}
	tok := &token.Token{AuthKey: authKey, KeyID: cfg.KeyID, TeamID: cfg.TeamID}
	return &Pusher{
		sandbox:    apns2.NewTokenClient(tok).Development(),
		production: apns2.NewTokenClient(tok).Production(),
		topic:      cfg.BundleID,
		devices:    devices,
		links:      links,
	}, nil
}

// Result is one delivery attempt, returned for the test endpoint.
type Result struct {
	Token  string `json:"token"`
	Env    string `json:"env"`
	Status int    `json:"status"`
	Reason string `json:"reason,omitempty"`
	ApnsID string `json:"apns_id,omitempty"`
}

// SendAlert pushes a to one person's phones and prunes dead tokens. Whether
// to push at all, and whether passively, was decided by the caller
// (internal/notify); this package only delivers.
func (p *Pusher) SendAlert(ctx context.Context, a model.Alert, person string, passive bool) []Result {
	devs, err := p.devices.ListDevices(ctx)
	if err != nil {
		slog.Error("list devices", "error", err)
		return nil
	}
	var out []Result
	for _, d := range devs {
		if d.Person == person {
			out = append(out, p.send(ctx, d.Token, d.Env, p.build(a, passive)))
		}
	}
	return out
}

// SendTest pushes a canned notification to one token, or to every phone of
// person when tok is empty.
func (p *Pusher) SendTest(ctx context.Context, tok, env, person string) []Result {
	pl := payload.NewPayload().AlertTitle("Sentinel").AlertBody("Test push from the homelab").Sound("default").
		Custom("kind", "test")
	if tok != "" {
		return []Result{p.send(ctx, tok, env, pl)}
	}
	devs, _ := p.devices.ListDevices(ctx)
	var out []Result
	for _, d := range devs {
		if d.Person == person {
			out = append(out, p.send(ctx, d.Token, d.Env, pl))
		}
	}
	return out
}

// Level names the interruption level a push for a would carry, for logs.
func Level(a model.Alert, passive bool) string { return string(interruptionLevel(a, passive)) }

// interruptionLevel is what the push tells iOS about how to present it. A
// source set to passive wins over everything: it is the user's choice for
// that source, so a passive camera stays passive. Otherwise critical and
// camera alerts are time-sensitive (they break through Focus) and the rest
// are active. Apple's "critical" level needs an entitlement Apple must
// approve; the app does not have it, so it is never sent.
func interruptionLevel(a model.Alert, passive bool) payload.EInterruptionLevel {
	switch {
	case passive:
		return payload.InterruptionLevelPassive
	case a.Severity == model.SeverityCritical || a.Kind == model.KindCamera:
		return payload.InterruptionLevelTimeSensitive
	}
	return payload.InterruptionLevelActive
}

func (p *Pusher) build(a model.Alert, passive bool) *payload.Payload {
	pl := payload.NewPayload().
		AlertTitle(a.Title).
		AlertBody(a.Body).
		ThreadID(a.Rule).
		MutableContent().
		InterruptionLevel(interruptionLevel(a, passive)).
		Custom("alert_id", a.ID).
		Custom("kind", a.Kind).
		Custom("severity", a.Severity)
	// A passive notification goes to Notification Center without a banner,
	// a sound or waking the screen, so it carries no sound. It still goes
	// at high priority: priority 5 is throttled and may never arrive, which
	// defeats "it is there when I look".
	if !passive {
		pl = pl.Sound("default")
	}
	if a.Link != "" {
		// The app registers a "dashboard" category with an "Open Dashboard"
		// action; the link is what that action opens.
		pl = pl.Category("dashboard").Custom("link", a.Link)
	}
	if p.links != nil {
		if u := p.links.ThumbURL(a); u != "" {
			pl = pl.Custom("thumb_url", u)
		}
	}
	return pl
}

func (p *Pusher) send(ctx context.Context, tok, env string, pl *payload.Payload) Result {
	client := p.production
	if env == model.EnvSandbox {
		client = p.sandbox
	}
	n := &apns2.Notification{
		DeviceToken: tok,
		Topic:       p.topic,
		Payload:     pl,
		PushType:    apns2.PushTypeAlert,
		Priority:    apns2.PriorityHigh,
		Expiration:  time.Now().Add(time.Hour),
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := client.PushWithContext(cctx, n)
	r := Result{Token: abbrev(tok), Env: env}
	if err != nil {
		r.Status, r.Reason = 0, err.Error()
		return r
	}
	r.Status, r.Reason, r.ApnsID = res.StatusCode, res.Reason, res.ApnsID
	if res.StatusCode == http.StatusGone || res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered {
		if err := p.devices.DeleteDevice(ctx, tok); err == nil {
			slog.Info("pruned dead device token", "token", abbrev(tok), "reason", res.Reason)
		}
	}
	return r
}

var errNoPush = errors.New("push disabled")

// Enabled reports whether a Pusher can send.
func (p *Pusher) Enabled() bool { return p != nil }

// ErrDisabled is returned by API handlers when push is not configured.
func ErrDisabled() error { return errNoPush }

func abbrev(tok string) string {
	if len(tok) <= 8 {
		return "…"
	}
	return tok[:4] + "…" + tok[len(tok)-4:]
}
