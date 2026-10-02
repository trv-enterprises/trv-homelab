// sentinel: homelab alerts to an iPhone.
//
// Subscribes to Marshal's sensors/alerts and Frigate's frigate/reviews,
// keeps alert history in SQLite, pushes new alerts through APNs, and serves
// a small API plus a media proxy for the trv-sentinel iOS app.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/api"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/config"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/ingest"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/push"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/rules"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

const (
	mediaLinkTTL  = 12 * time.Hour
	sweepInterval = time.Hour
	minStaleAfter = 6 * time.Hour
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	rulesReader := rules.NewReader(cfg.RulesPath, 5*time.Minute)
	signer := api.NewSigner(cfg.PublicBaseURL, cfg.APIToken, mediaLinkTTL)

	pusher, err := push.New(push.Config{Key: cfg.APNSKey, KeyID: cfg.APNSKeyID, TeamID: cfg.APNSTeamID, BundleID: cfg.APNSBundleID}, st, signer)
	if err != nil {
		slog.Error("apns", "error", err)
		os.Exit(1)
	}
	if pusher == nil {
		slog.Warn("push disabled: APNS_* not fully configured")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hub := api.NewHub(func(ev model.Event) {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		pusher.Notify(pctx, ev)
	})

	engine := ingest.New(cfg.MQTTBroker, cfg.MQTTClientID, st, hub, rulesReader)

	srv := api.New(api.Deps{
		Token: cfg.APIToken, Store: st, Rules: rulesReader, Pusher: pusher, Engine: engine,
		Signer: signer, Hub: hub, FrigateURL: cfg.FrigateURL,
	})
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: SSE and video responses are long-lived by design.
	}

	go func() {
		slog.Info("http listening", "addr", cfg.HTTPAddr, "public", cfg.PublicBaseURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http", "error", err)
			stop()
		}
	}()

	go sweeper(ctx, st, rulesReader, hub, cfg.Retention)

	if err := engine.Run(ctx); err != nil {
		slog.Error("mqtt", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	slog.Info("stopped")
}

// sweeper runs the two housekeeping jobs: staleness (a sensor alert whose
// rule has gone quiet past its repeat window, which happens when Marshal
// restarts and forgets it) and retention pruning.
func sweeper(ctx context.Context, st *store.Store, rr *rules.Reader, hub *api.Hub, retention time.Duration) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			active, err := st.ActiveSensorRules(ctx)
			if err != nil {
				slog.Error("sweep: active rules", "error", err)
				continue
			}
			for rule := range active {
				window := 2 * rr.RepeatWindow(rule)
				if window < minStaleAfter {
					window = minStaleAfter
				}
				stale, err := st.MarkStale(ctx, rule, now.Add(-window), now)
				if err != nil {
					slog.Error("sweep: mark stale", "rule", rule, "error", err)
					continue
				}
				for _, a := range stale {
					slog.Info("alert marked stale", "rule", rule, "id", a.ID)
					hub.Broadcast(model.Event{Type: "updated", Alert: a})
				}
			}
			if n, err := st.Prune(ctx, now.Add(-retention)); err != nil {
				slog.Error("sweep: prune", "error", err)
			} else if n > 0 {
				slog.Info("pruned old alerts", "count", n)
			}
		}
	}
}
