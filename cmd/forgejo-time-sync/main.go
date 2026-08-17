// Command forgejo-time-sync polls a Forgejo repo's tracked time and pushes
// any entry not already recorded in local state into Toggl Track, on a
// timer, until it receives SIGINT or SIGTERM.
//
// Every setting is an environment variable — see the README — so there is
// no flag parsing here, and nothing to hand off to a CLI framework.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/config"
	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/alrayyes/forgejo-time-sync/internal/state"
	"github.com/alrayyes/forgejo-time-sync/internal/sync"
	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

// reconcileLookback is how far back to search Toggl when seeding an empty
// state file on a cold start. Wide enough to catch a long-idle repo without
// costing more than the one-time reconciliation pass this happens in.
const reconcileLookback = 90 * 24 * time.Hour

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(context.Background(), logger); err != nil {
		logger.Error("forgejo-time-sync exiting", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	st, err := state.Load(cfg.StateFilePath)
	if err != nil {
		return err
	}

	fg := forgejo.Client{BaseURL: cfg.ForgejoBaseURL, Token: cfg.ForgejoToken}
	tg := toggl.NewClient(cfg.TogglAPIToken, cfg.TogglMaxRequestsPerHour)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	projectAlreadyResolved := cfg.TogglProjectID != 0 || st.ProjectID() != 0
	projectID, err := sync.ResolveProject(ctx, tg, st, cfg.ForgejoOwner, cfg.ForgejoRepo, cfg.TogglWorkspaceID, cfg.TogglProjectID)
	if err != nil {
		return err
	}
	if !projectAlreadyResolved {
		logger.Info("auto-provisioned toggl client/project", "toggl_project_id", projectID)
	}

	if st.Len() == 0 {
		logger.Info("state is empty, reconciling against toggl before starting the poll loop")
		if err := sync.Reconcile(ctx, tg, st, time.Now().Add(-reconcileLookback)); err != nil {
			logger.Warn("reconciliation failed, continuing with empty state", "error", err)
		}
	}

	logger.Info("starting poll loop",
		"interval", cfg.SyncInterval,
		"forgejo_owner", cfg.ForgejoOwner,
		"forgejo_repo", cfg.ForgejoRepo,
	)

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil
		case <-ticker.C:
			poll(ctx, logger, fg, tg, st, cfg, projectID)
		}
	}
}

func poll(ctx context.Context, logger *slog.Logger, fg forgejo.Client, tg *toggl.Client, st *state.State, cfg config.Config, projectID int64) {
	created, err := sync.RepoTimes(ctx, fg, tg, st, cfg.ForgejoOwner, cfg.ForgejoRepo, cfg.TogglWorkspaceID, projectID)
	if err != nil {
		logger.Error("sync pass failed, retrying next poll", "error", err)
		return
	}
	if len(created) > 0 {
		logger.Info("synced new time entries", "count", len(created))
	}
}
