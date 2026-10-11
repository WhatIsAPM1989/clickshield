package main

import (
	"context"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/replay"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/store"
)

func bootstrap(ctx context.Context, st *store.Store, cfg config.Config) error {
	if cfg.ReplayS3.Endpoint != "" {
		objects, err := replay.NewS3(cfg.ReplayS3)
		if err != nil {
			return err
		}
		if err = objects.EnsureBucket(ctx); err != nil {
			return err
		}
	}
	if err := st.EnsureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}
	p, err := st.EnsureProject(ctx, cfg.BootstrapProjectName, cfg.BootstrapProjectKey, cfg.BootstrapAllowedOrigins, scoring.DefaultConfig())
	if err != nil {
		return err
	}
	cipher, err := cryptox.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	for _, item := range []struct{ provider, id, secret, idField, secretField string }{
		{"YANDEX_METRICA", cfg.YandexCounterID, cfg.YandexMeasurementToken, "counter_id", "measurement_token"},
		{"GOOGLE_ANALYTICS", cfg.GoogleMeasurementID, cfg.GoogleAPISecret, "measurement_id", "api_secret"},
	} {
		if item.id == "" || item.secret == "" {
			continue
		}
		secret, err := cipher.EncryptJSON(map[string]any{item.secretField: item.secret})
		if err != nil {
			return err
		}
		if err = st.EnsureIntegration(ctx, p.ID, item.provider, map[string]any{item.idField: item.id}, secret); err != nil {
			return err
		}
	}
	return nil
}
