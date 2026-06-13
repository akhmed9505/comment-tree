// Package main provides the entry point for the Backforge server.
//
// It initializes the application, sets up signal handling for graceful shutdown,
// and runs the HTTP server.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/akhmed9505/comment-tree/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx)
	if err != nil {
		log.Printf("failed to create app: %v", err)
		return
	}

	if err := a.Run(ctx); err != nil {
		a.Logger.Error("app stopped with error", "err", err)
	}
}
