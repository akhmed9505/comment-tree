package main

import (
	"fmt"
	"os"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load configuration: %v\n", err)
		os.Exit(1)
	}
	log, err := logger.New(cfg.Logging, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize logger: %v\n", err)
		os.Exit(1)
	}
	log.Info("application initialized")
}
