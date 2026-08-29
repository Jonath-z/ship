// Command worker consumes jobs from Redis and performs all long-running work:
// server provisioning and inspection, deployments through Kamal, rollbacks,
// health checks, and metric collection (spec §51).
//
// This is the only binary that shells out to Kamal or opens SSH connections.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("ship-worker starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go run(ctx, logger)

	<-stop
	logger.Info("shutting down")
	cancel()
	time.Sleep(time.Second) // let in-flight handlers unwind
}

func run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// TODO(SH-071): dequeue from Redis, dispatch to a job handler,
			// honour per-environment locks so two deploys never overlap.
			logger.Debug("polling job queue")
		}
	}
}
