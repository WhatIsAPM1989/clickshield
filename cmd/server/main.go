package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], log); err != nil {
		log.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log *slog.Logger) error {
	command, rest, err := parseCommand(args)
	if err != nil {
		return err
	}
	if command == "help" {
		fmt.Println("clickshield serve | worker | migrate | bootstrap | rescore SESSION_ID... | reproduce CLASSIFICATION_ID | backfill traffic|networks|bots|features|analytics|replay")
		return nil
	}
	cfg, err := config.LoadFor(command)
	if err != nil {
		return err
	}
	var st *store.Store
	if command == "migrate" || command == "backfill" {
		st, err = store.NewDatabase(ctx, cfg.DatabaseURL)
	} else {
		st, err = store.New(ctx, cfg.DatabaseURL, cfg.SessionSecret)
	}
	if err != nil {
		return err
	}
	defer st.Close()
	switch command {
	case "migrate":
		return st.Migrate(ctx)
	case "bootstrap":
		return bootstrap(ctx, st, cfg)
	case "backfill":
		return backfill(ctx, st, cfg, rest[0], log)
	case "rescore":
		return rescore(ctx, st, cfg, rest, log)
	case "reproduce":
		return reproduce(ctx, st, rest[0])
	case "worker":
		return worker(ctx, st, cfg, log)
	case "serve":
		return serve(ctx, st, cfg, log)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func parseCommand(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "serve", nil, nil
	}
	command, rest := args[0], args[1:]
	if command == "--help" || command == "-h" {
		command = "help"
	}
	if command == "--rescore" {
		command = "rescore"
	}
	switch command {
	case "serve", "worker", "migrate", "bootstrap", "help":
		if len(rest) != 0 {
			return "", nil, fmt.Errorf("%s does not accept arguments", command)
		}
	case "rescore":
		if len(rest) == 0 {
			return "", nil, fmt.Errorf("rescore requires session IDs")
		}
		for _, id := range rest {
			if !store.ValidUUID(id) {
				return "", nil, fmt.Errorf("invalid session ID %q", id)
			}
		}
	case "reproduce":
		if len(rest) != 1 {
			return "", nil, fmt.Errorf("reproduce requires a classification ID")
		}
		if n, err := strconv.ParseInt(rest[0], 10, 64); err != nil || n < 1 {
			return "", nil, fmt.Errorf("invalid classification ID")
		}
	case "backfill":
		if len(rest) != 1 || (rest[0] != "traffic" && rest[0] != "networks" && rest[0] != "bots" && rest[0] != "features" && rest[0] != "analytics" && rest[0] != "replay") {
			return "", nil, fmt.Errorf("backfill requires traffic, networks, bots, features, analytics or replay")
		}
	default:
		return "", nil, fmt.Errorf("unknown command %q; use --help", command)
	}
	return command, rest, nil
}
