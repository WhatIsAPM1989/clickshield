package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lamooof/clickshield/internal/asn"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/geo"
	"github.com/lamooof/clickshield/internal/httpapi"
	"github.com/lamooof/clickshield/internal/integration"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/service"
	"github.com/lamooof/clickshield/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	st, err := store.New(ctx, cfg.DatabaseURL, cfg.SessionSecret)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err = st.Migrate(ctx); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if err = st.BackfillTraffic(ctx); err != nil {
		log.Error("traffic backfill failed", "error", err)
		os.Exit(1)
	}
	if cfg.GeoCityDatabase != "" {
		if err = geo.Load(cfg.GeoCityDatabase); err != nil {
			log.Error("GeoIP city database failed", "error", err)
			os.Exit(1)
		}
		defer geo.Close()
		log.Info("GeoIP city database loaded")
	}
	if ranges, err := asn.Load(cfg.ASNDatabase); err != nil {
		log.Warn("ASN database not loaded; networks are taken from proxy headers only", "path", cfg.ASNDatabase, "error", err)
	} else {
		log.Info("ASN database loaded", "ranges", ranges)
		if filled, requeued, err := st.FillNetworks(ctx); err != nil {
			log.Error("network labeling failed", "error", err)
		} else if filled > 0 {
			log.Info("sessions labeled with networks", "filled", filled, "requeued", requeued)
		}
	}
	if labeled, requeued, err := st.LabelBots(ctx); err != nil {
		log.Error("bot labeling failed", "error", err)
	} else if labeled > 0 {
		log.Info("sessions labeled with known bots", "labeled", labeled, "requeued", requeued)
	}
	if len(os.Args) > 1 {
		if os.Args[1] != "--rescore" || len(os.Args) < 3 {
			log.Error("usage: clickshield --rescore SESSION_ID [SESSION_ID...]")
			os.Exit(2)
		}
		svc := service.New(st, nil, cfg, log)
		for _, id := range os.Args[2:] {
			if err = svc.Finalize(ctx, id); err != nil {
				log.Error("rescore failed", "session_id", id, "error", err)
				os.Exit(1)
			}
			log.Info("session rescored", "session_id", id)
		}
		return
	}
	if err = st.EnsureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Error("admin bootstrap failed", "error", err)
		os.Exit(1)
	}
	cipher, err := cryptox.New(cfg.EncryptionKey)
	if err != nil {
		log.Error("cipher init failed", "error", err)
		os.Exit(1)
	}
	project, err := st.EnsureProject(ctx, cfg.BootstrapProjectName, cfg.BootstrapProjectKey, cfg.BootstrapAllowedOrigins, scoring.DefaultConfig())
	if err != nil {
		log.Error("project bootstrap failed", "error", err)
		os.Exit(1)
	}
	if cfg.YandexCounterID != "" && cfg.YandexMeasurementToken != "" {
		secret, _ := cipher.EncryptJSON(map[string]any{"measurement_token": cfg.YandexMeasurementToken})
		if err = st.UpsertIntegration(ctx, project.ID, "YANDEX_METRICA", true, map[string]any{"counter_id": cfg.YandexCounterID}, secret); err != nil {
			log.Error("Yandex integration bootstrap failed", "error", err)
		}
	}
	if cfg.GoogleMeasurementID != "" && cfg.GoogleAPISecret != "" {
		secret, _ := cipher.EncryptJSON(map[string]any{"api_secret": cfg.GoogleAPISecret})
		if err = st.UpsertIntegration(ctx, project.ID, "GOOGLE_ANALYTICS", true, map[string]any{"measurement_id": cfg.GoogleMeasurementID}, secret); err != nil {
			log.Error("Google integration bootstrap failed", "error", err)
		}
	}
	sender := integration.New(cipher)
	svc := service.New(st, sender, cfg, log)
	svc.Run(ctx)
	api := httpapi.New(cfg, st, svc, cipher, log)
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Info("ClickShield started", "addr", cfg.HTTPAddr, "project_key", project.PublicKey)
		if e := httpServer.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Error("HTTP server failed", "error", e)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdown, c := context.WithTimeout(context.Background(), 15*time.Second)
	defer c()
	if err = httpServer.Shutdown(shutdown); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
}
