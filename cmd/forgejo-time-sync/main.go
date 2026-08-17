// Command forgejo-time-sync polls a Forgejo repo's tracked time and pushes
// any entry not already recorded in local state into Toggl Track, on a
// timer, until it receives SIGINT or SIGTERM.
//
// Every setting is an environment variable — see the README — so there is
// no flag parsing here, and nothing to hand off to a CLI framework. The one
// exception is a single positional "healthcheck" argument, invoked by the
// Dockerfile's HEALTHCHECK rather than a person, since the distroless base
// image has no shell or curl to run one out of.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/config"
	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/alrayyes/forgejo-time-sync/internal/health"
	"github.com/alrayyes/forgejo-time-sync/internal/state"
	"github.com/alrayyes/forgejo-time-sync/internal/sync"
	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

// reconcileLookback is how far back to search Toggl when seeding an empty
// state file on a cold start. Wide enough to catch a long-idle repo without
// costing more than the one-time reconciliation pass this happens in.
const reconcileLookback = 90 * 24 * time.Hour

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(context.Background(), logger); err != nil {
		logger.Error("forgejo-time-sync exiting", "error", err)
		os.Exit(1)
	}
}

// runHealthcheck reads the same env config the main loop would, so it's
// checking the heartbeat file that same configuration's poll loop writes.
func runHealthcheck() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	hb := health.NewHeartbeat(heartbeatPath(cfg.StateFilePath))
	return hb.Check(heartbeatMaxAge(cfg.SyncInterval))
}

// heartbeatPath keeps the heartbeat file on the same volume as the state
// file, so it needs no config of its own.
func heartbeatPath(stateFilePath string) string {
	return filepath.Join(filepath.Dir(stateFilePath), "healthcheck")
}

// heartbeatMaxAge allows a couple of missed ticks — a single slow poll
// (a slow Forgejo/Toggl response, not a hang) shouldn't flap the container
// unhealthy — while still catching a genuinely stuck loop.
func heartbeatMaxAge(interval time.Duration) time.Duration {
	return 3 * interval
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

	hb := health.NewHeartbeat(heartbeatPath(cfg.StateFilePath))
	touchHeartbeat := func() {
		if err := hb.Touch(); err != nil {
			logger.Warn("failed to update healthcheck heartbeat", "error", err)
		}
	}
	// Touched once up front so the healthcheck can pass immediately,
	// rather than waiting out the first full interval.
	touchHeartbeat()

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil
		case <-ticker.C:
			poll(ctx, logger, fg, tg, st, cfg, projectID)
			// Touched regardless of whether poll found or synced
			// anything — this tracks that the loop is still cycling,
			// not that Forgejo/Toggl are reachable. A real outage on
			// either side should keep retrying, not get "fixed" by
			// Docker restarting a container that was never stuck.
			touchHeartbeat()
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
