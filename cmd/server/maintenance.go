package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lamooof/clickshield/internal/analytics"
	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/replay"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/store"
	"log/slog"
	"os"
	"strconv"
)

func backfill(ctx context.Context, st *store.Store, cfg config.Config, kind string, log *slog.Logger) error {
	if kind == "replay" {
		objects, err := replayObjects(cfg)
		if err != nil {
			return err
		}
		return replay.NewRepository(st.Pool).MigrateLegacy(ctx, objects)
	}
	if kind == "features" {
		return st.BackfillFeatures(ctx)
	}
	if kind == "analytics" {
		if _, err := st.Pool.Exec(ctx, `INSERT INTO analytics_projection_outbox(session_id,project_id) SELECT id,project_id FROM sessions`); err != nil {
			return err
		}
		projector := analytics.NewProjector(st.Pool)
		for {
			ok, err := projector.ProjectOne(ctx)
			if err != nil {
				return err
			}
			if !ok {
				var pending int64
				if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM analytics_projection_outbox`).Scan(&pending); err != nil {
					return err
				}
				if pending != 0 {
					return fmt.Errorf("%d analytics projections remain claimed or deferred; stop concurrent workers and retry", pending)
				}
				return nil
			}
		}
	}
	if kind == "traffic" {
		return st.BackfillTraffic(ctx)
	}
	if err := loadLookups(cfg, log); err != nil {
		return err
	}
	var count, requeued int64
	var err error
	if kind == "networks" {
		count, requeued, err = st.FillNetworks(ctx)
	} else {
		count, requeued, err = st.LabelBots(ctx)
	}
	log.Info("backfill finished", "kind", kind, "updated", count, "requeued", requeued)
	return err
}

func rescore(ctx context.Context, st *store.Store, cfg config.Config, ids []string, log *slog.Logger) error {
	if err := loadLookups(cfg, log); err != nil {
		return err
	}
	scorer := antifraud.NewScorer(st)
	for _, id := range ids {
		if err := scorer.Finalize(ctx, id); err != nil {
			return err
		}
		log.Info("session rescored", "session_id", id)
	}
	return nil
}

func reproduce(ctx context.Context, st *store.Store, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 {
		return fmt.Errorf("invalid classification ID")
	}
	w, err := st.ScoreHistory(ctx, n)
	if err != nil {
		return err
	}
	if w.Version != antifraud.AlgorithmVersion {
		return fmt.Errorf("algorithm %q cannot be reproduced by this binary", w.Version)
	}
	score := scoring.Score(w.Input.Context, w.Features, scoring.Parse(w.Config))
	return json.NewEncoder(os.Stdout).Encode(score)
}
