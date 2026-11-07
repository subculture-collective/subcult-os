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

// listmonk-relay sends due listmonk announcement deliveries through the
// configured listmonk. It refuses to run unless LISTMONK_URL,
// LISTMONK_API_USER and LISTMONK_API_TOKEN are set. See docs/development/listmonk-relay.md.
func main() {
	watch := flag.Bool("watch", false, "repeat bounded batches every 30 seconds")
	limit := flag.Int("limit", 10, "maximum deliveries per batch (1-100)")
	flag.Parse()
	if flag.NArg() != 0 || *limit < 1 || *limit > 100 {
		fmt.Fprintln(os.Stderr, "usage: listmonk-relay [-watch] [-limit 1..100]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config := app.LoadConfig()
	if err := config.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration; inspect LISTMONK_* settings")
		os.Exit(1)
	}
	if config.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := app.OpenDB(ctx, config.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay database unavailable")
		os.Exit(1)
	}
	defer db.Close()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, time.Duration(*limit)*20*time.Second+30*time.Second)
		report, err := app.RunListmonkRelay(batchCtx, config, db, *limit, nil)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "listmonk relay batch failed; inspect configuration and delivery status")
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
