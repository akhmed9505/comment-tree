// Package logger provides application-specific logger initialization.
package logger

import (
	"github.com/akhmed9505/comment-tree/internal/config"

	"github.com/wb-go/wbf/logger"
)

// New creates a zerolog adapter configured for the application environment.
func New(cfg *config.Config) *logger.ZerologAdapter {
	return logger.NewZerologAdapter(cfg.Env, cfg.Env)
}
