package main

import (
	"context"
	"errors"
	"github.com/lamooof/clickshield/internal/analytics"
	"github.com/lamooof/clickshield/internal/asn"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/geo"
	"github.com/lamooof/clickshield/internal/httpapi"
	"github.com/lamooof/clickshield/internal/integration"
	"github.com/lamooof/clickshield/internal/replay"
	"github.com/lamooof/clickshield/internal/service"
	"github.com/lamooof/clickshield/internal/store"
	"log/slog"
	"net/http"
	"time"
)

func loadLookups(cfg config.Config, log *slog.Logger) error {
	if cfg.GeoCityDatabase != "" {
		if err := geo.Load(cfg.GeoCityDatabase); err != nil {
			return err
		}
	}
	if _, err := asn.Load(cfg.ASNDatabase); err != nil {
		log.Warn("ASN database not loaded", "error", err)
	}
	return nil
}

func newService(st *store.Store, cfg config.Config, log *slog.Logger, options ...service.WorkerOptions) (*service.Service, *cryptox.Cipher, error) {
	cipher, err := cryptox.New(cfg.EncryptionKey)
	if err != nil {
		return nil, nil, err
	}
	return service.New(st, integration.New(cipher), cfg, log, options...), cipher, nil
}

func serve(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	if err := st.CheckSchema(ctx); err != nil {
		return err
	}
	if err := replay.NewRepository(st.Pool).CheckLegacy(ctx); err != nil {
		return err
	}
	if err := loadLookups(cfg, log); err != nil {
		return err
	}
	defer geo.Close()
	svc, cipher, err := newService(st, cfg, log)
	if err != nil {
		return err
	}
	reader, err := analytics.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer reader.Close()
	objects, err := replayObjects(cfg)
	if err != nil {
		return err
	}
	api := httpapi.New(cfg, st, svc, cipher, log, httpapi.Dependencies{Analytics: reader, Replay: replay.NewService(replay.NewRepository(st.Pool), objects)})
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	result := make(chan error, 1)
	go func() { log.Info("HTTP server started", "addr", cfg.HTTPAddr); result <- server.ListenAndServe() }()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func worker(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	if err := st.CheckSchema(ctx); err != nil {
		return err
	}
	if err := replay.NewRepository(st.Pool).CheckLegacy(ctx); err != nil {
		return err
	}
	if err := loadLookups(cfg, log); err != nil {
		return err
	}
	defer geo.Close()
	objects, err := replayObjects(cfg)
	if err != nil {
		return err
	}
	svc, _, err := newService(st, cfg, log, service.WorkerOptions{Replay: replay.NewRepository(st.Pool), Objects: objects})
	if err != nil {
		return err
	}
	svc.Run(ctx)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return svc.Wait(shutdown)
}

func replayObjects(cfg config.Config) (replay.Objects, error) {
	if cfg.ReplayS3.Endpoint == "" {
		return nil, nil
	}
	return replay.NewS3(cfg.ReplayS3)
}
