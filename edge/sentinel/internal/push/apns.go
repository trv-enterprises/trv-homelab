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
	IsMuted(ctx context.Context, rule string, now time.Time) (bool, error)
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

// Notify decides whether an event deserves a push and sends it to every
// registered device. Rules: first sighting of any alert; repeats only when
// critical. Muted rules are skipped.
func (p *Pusher) Notify(ctx context.Context, ev model.Event) {
	if p == nil {
		return
	}
	a := ev.Alert
	if !ev.Created && !(a.Kind == model.KindSensor && a.Severity == model.SeverityCritical && ev.Type == "updated" && a.Status == model.StatusActive) {
		return
	}
	if ev.Created || a.RepeatCount > 0 {
		if muted, err := p.devices.IsMuted(ctx, a.Rule, time.Now()); err == nil && muted {
			slog.Info("push suppressed: rule muted", "rule", a.Rule)
			return
		}
	}
	for _, r := range p.SendAlert(ctx, a) {
		if r.Status == http.StatusOK {
			slog.Info("push sent", "alert", a.ID, "env", r.Env, "apns_id", r.ApnsID)
		} else {
			slog.Warn("push failed", "alert", a.ID, "env", r.Env, "status", r.Status, "reason", r.Reason)
		}
	}
}

// SendAlert pushes a to all devices and prunes dead tokens.
func (p *Pusher) SendAlert(ctx context.Context, a model.Alert) []Result {
	devs, err := p.devices.ListDevices(ctx)
	if err != nil {
		slog.Error("list devices", "error", err)
		return nil
	}
	var out []Result
	for _, d := range devs {
		out = append(out, p.send(ctx, d.Token, d.Env, p.build(a)))
	}
	return out
}

// SendTest pushes a canned notification to one token (or all devices when
// tok is empty).
func (p *Pusher) SendTest(ctx context.Context, tok, env string) []Result {
	pl := payload.NewPayload().AlertTitle("Sentinel").AlertBody("Test push from the homelab").Sound("default").
		Custom("kind", "test")
	if tok != "" {
		return []Result{p.send(ctx, tok, env, pl)}
	}
	devs, _ := p.devices.ListDevices(ctx)
	var out []Result
	for _, d := range devs {
		out = append(out, p.send(ctx, d.Token, d.Env, pl))
	}
	return out
}

func (p *Pusher) build(a model.Alert) *payload.Payload {
	pl := payload.NewPayload().
		AlertTitle(a.Title).
		AlertBody(a.Body).
		Sound("default").
		ThreadID(a.Rule).
		MutableContent().
		Custom("alert_id", a.ID).
		Custom("kind", a.Kind).
		Custom("severity", a.Severity)
	if a.Severity == model.SeverityCritical || a.Kind == model.KindCamera {
		pl = pl.InterruptionLevel(payload.InterruptionLevelTimeSensitive)
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
