package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/subcult-os/internal/app"
)

// backline-handoff delivers approved announcement handoffs to the configured
// Backline receiver. It refuses to run unless BACKLINE_HANDOFF_URL and
// BACKLINE_HANDOFF_SECRET are both set. See docs/development/backline-handoff.md.
func main() {
	watch := flag.Bool("watch", false, "repeat bounded batches every 30 seconds")
	limit := flag.Int("limit", 10, "maximum handoffs per batch (1-100)")
	flag.Parse()
	if flag.NArg() != 0 || *limit < 1 || *limit > 100 {
		fmt.Fprintln(os.Stderr, "usage: backline-handoff [-watch] [-limit 1..100]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config := app.LoadConfig()
	if err := config.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration; inspect BACKLINE_HANDOFF_* settings")
		os.Exit(1)
	}
	if config.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := app.OpenDB(ctx, config.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handoff database unavailable")
		os.Exit(1)
	}
	defer db.Close()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, time.Duration(*limit)*20*time.Second+30*time.Second)
		report, err := app.RunBacklineHandoffs(batchCtx, config, db, *limit, nil)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "backline handoff batch failed; inspect configuration and handoff status")
			os.Exit(1)
		}
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			os.Exit(1)
		}
		if !*watch {
			return
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
