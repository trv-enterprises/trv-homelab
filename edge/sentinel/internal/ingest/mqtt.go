// Package ingest owns the MQTT side of sentinel: one paho client, a bounded
// inbox drained by a single dispatcher, and the two topic handlers that turn
// Marshal and Frigate messages into store changes and events.
//
// The client settings follow the pitfalls recorded in trv-marshal/CLAUDE.md
// (non-blocking subscribe callback, explicit resubscribe on every connect,
// bounded token waits, keepalive 30s) with one deliberate difference: this
// client uses a persistent session (CleanSession=false, fixed client id,
// QoS 1) so Mosquitto queues sensors/alerts and frigate/reviews while
// sentinel is down. Marshal cannot do that because its recovery path builds
// a brand-new client; sentinel is a single compose instance that restarts
// in place, so a fixed id never races itself.
package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

const (
	TopicSensorAlerts  = "sensors/alerts"
	TopicFrigateReview = "frigate/reviews"
	TopicHeartbeat     = "sentinel/heartbeat"

	inboxSize         = 256
	heartbeatInterval = 60 * time.Second
	// stallTimeout is how long the heartbeat may go unseen before the client
	// is declared deaf and rebuilt. The heartbeat is our own publish looped
	// back by the broker, so it is a true end-to-end check.
	stallTimeout = 5 * time.Minute
	tokenWait    = 10 * time.Second
)

type inbound struct {
	topic   string
	payload []byte
	at      time.Time
}

// Sink receives every store change. The api layer fans it out over SSE and
// the push layer decides whether to notify.
type Sink interface {
	OnEvent(ev model.Event)
}

type Engine struct {
	broker   string
	clientID string
	store    *store.Store
	sink     Sink

	inbox chan inbound

	mu       sync.Mutex
	client   mqtt.Client
	lastSeen time.Time // last heartbeat loopback
	resub    sync.Mutex
	resubbed bool

	stats struct {
		sync.Mutex
		received, dropped, handled, errors uint64
	}
}

func New(broker, clientID string, st *store.Store, sink Sink) *Engine {
	return &Engine{broker: broker, clientID: clientID, store: st, sink: sink, inbox: make(chan inbound, inboxSize)}
}

// Run connects and blocks until ctx is done. It owns the dispatcher, the
// heartbeat publisher and the stall watchdog.
func (e *Engine) Run(ctx context.Context) error {
	go e.dispatch(ctx)
	if err := e.connect(); err != nil {
		return err
	}
	go e.heartbeat(ctx)
	<-ctx.Done()
	e.mu.Lock()
	c := e.client
	e.mu.Unlock()
	if c != nil {
		c.Disconnect(250)
	}
	return nil
}

func (e *Engine) connect() error {
	opts := mqtt.NewClientOptions().
		AddBroker(e.broker).
		SetClientID(e.clientID).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetMaxReconnectInterval(60 * time.Second).
		SetOrderMatters(false).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			slog.Error("mqtt connection lost", "error", err)
		}).
		SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
			slog.Warn("mqtt reconnecting")
		}).
		SetOnConnectHandler(func(c mqtt.Client) {
			slog.Info("mqtt connected")
			// Subscribe on every connect. With a persistent session the
			// broker remembers the subscriptions, but re-asserting is
			// idempotent and covers the first connect and a broker whose
			// persistence was wiped. The guard collapses overlapping
			// OnConnect calls, which paho can issue from two goroutines.
			e.resub.Lock()
			defer e.resub.Unlock()
			if err := e.subscribe(c); err != nil {
				slog.Error("subscribe failed", "error", err)
			}
		})

	c := mqtt.NewClient(opts)
	tok := c.Connect()
	if !tok.WaitTimeout(30 * time.Second) {
		return fmt.Errorf("mqtt connect: timeout")
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	e.mu.Lock()
	e.client = c
	e.lastSeen = time.Now()
	e.mu.Unlock()
	return nil
}

func (e *Engine) subscribe(c mqtt.Client) error {
	filters := map[string]byte{TopicSensorAlerts: 1, TopicFrigateReview: 1, TopicHeartbeat: 0}
	tok := c.SubscribeMultiple(filters, e.onMessage)
	if !tok.WaitTimeout(tokenWait) {
		return fmt.Errorf("subscribe timeout")
	}
	return tok.Error()
}

// onMessage runs on paho's router goroutine: never block here. Copy the
// payload (paho reuses buffers) and enqueue; drop with a counter if the
// dispatcher is behind.
func (e *Engine) onMessage(_ mqtt.Client, m mqtt.Message) {
	e.stats.Lock()
	e.stats.received++
	e.stats.Unlock()
	if m.Topic() == TopicHeartbeat {
		e.mu.Lock()
		e.lastSeen = time.Now()
		e.mu.Unlock()
		return
	}
	p := make([]byte, len(m.Payload()))
	copy(p, m.Payload())
	select {
	case e.inbox <- inbound{topic: m.Topic(), payload: p, at: time.Now()}:
	default:
		e.stats.Lock()
		e.stats.dropped++
		e.stats.Unlock()
		slog.Warn("inbox full, dropping message", "topic", m.Topic())
	}
}

func (e *Engine) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-e.inbox:
			hctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := e.handle(hctx, in)
			cancel()
			e.stats.Lock()
			if err != nil {
				e.stats.errors++
			} else {
				e.stats.handled++
			}
			e.stats.Unlock()
			if err != nil {
				slog.Warn("message not handled", "topic", in.topic, "error", err)
			}
		}
	}
}

func (e *Engine) handle(ctx context.Context, in inbound) error {
	switch in.topic {
	case TopicSensorAlerts:
		return e.handleSensor(ctx, in)
	case TopicFrigateReview:
		return e.handleReview(ctx, in)
	}
	return nil
}

func (e *Engine) handleSensor(ctx context.Context, in inbound) error {
	a, msg, err := parseSensor(in.payload, in.at)
	if err != nil {
		return err
	}
	switch a.Type {
	case "resolved":
		alert, ok, err := e.store.ResolveByRule(ctx, msg.Rule, msg.At, msg.Raw)
		if err != nil {
			return err
		}
		if !ok {
			slog.Info("resolved with no active alert", "rule", msg.Rule)
			return nil
		}
		e.emit(model.Event{Type: "updated", Alert: alert})
	default:
		alert, created, err := e.store.UpsertSensor(ctx, msg, a.Type == "repeat")
		if err != nil {
			return err
		}
		typ := "updated"
		if created {
			typ = "created"
		}
		e.emit(model.Event{Type: typ, Alert: alert, Created: created})
	}
	return nil
}

func (e *Engine) handleReview(ctx context.Context, in inbound) error {
	_, msg, err := parseReview(in.payload, in.at)
	if err != nil {
		return err
	}
	alert, created, ok, err := e.store.UpsertReview(ctx, msg)
	if err != nil {
		return err
	}
	if !ok {
		return nil // detection-only review, not promoted
	}
	typ := "updated"
	if created {
		typ = "created"
	}
	e.emit(model.Event{Type: typ, Alert: alert, Created: created})
	return nil
}

func (e *Engine) emit(ev model.Event) {
	if e.sink != nil {
		e.sink.OnEvent(ev)
	}
}

// heartbeat publishes to our own topic and rebuilds the client if the
// loopback has not been seen for stallTimeout. IsConnected() is not trusted
// as a liveness signal (it reports intent, not socket health).
func (e *Engine) heartbeat(ctx context.Context) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			e.mu.Lock()
			c := e.client
			last := e.lastSeen
			e.mu.Unlock()
			if c != nil && c.IsConnectionOpen() {
				tok := c.Publish(TopicHeartbeat, 0, false, now.UTC().Format(time.RFC3339))
				tok.WaitTimeout(tokenWait)
			}
			if now.Sub(last) > stallTimeout {
				slog.Error("mqtt stalled: no heartbeat loopback", "since", last)
				e.recover()
			}
		}
	}
}

func (e *Engine) recover() {
	e.mu.Lock()
	old := e.client
	e.client = nil
	e.mu.Unlock()
	if old != nil {
		old.Disconnect(250)
	}
	if err := e.connect(); err != nil {
		slog.Error("mqtt recovery failed", "error", err)
		return
	}
	slog.Info("mqtt client rebuilt")
}

// Stats is a point-in-time snapshot for /healthz.
type Stats struct {
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_heartbeat"`
	Received  uint64    `json:"received"`
	Handled   uint64    `json:"handled"`
	Dropped   uint64    `json:"dropped"`
	Errors    uint64    `json:"errors"`
}

func (e *Engine) Stats() Stats {
	e.mu.Lock()
	c, last := e.client, e.lastSeen
	e.mu.Unlock()
	e.stats.Lock()
	defer e.stats.Unlock()
	return Stats{
		Connected: c != nil && c.IsConnectionOpen(),
		LastSeen:  last,
		Received:  e.stats.received, Handled: e.stats.handled, Dropped: e.stats.dropped, Errors: e.stats.errors,
	}
}

// Healthy is true when connected and the heartbeat loopback is recent.
func (e *Engine) Healthy() bool {
	s := e.Stats()
	return s.Connected && time.Since(s.LastSeen) < stallTimeout
}

// Publish sends a message (used by phase 3 for Marshal enable topics).
func (e *Engine) Publish(topic string, qos byte, retained bool, payload string) error {
	e.mu.Lock()
	c := e.client
	e.mu.Unlock()
	if c == nil {
		return fmt.Errorf("mqtt not connected")
	}
	tok := c.Publish(topic, qos, retained, payload)
	if !tok.WaitTimeout(tokenWait) {
		return fmt.Errorf("publish timeout")
	}
	return tok.Error()
}
