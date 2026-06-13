// Package app wires together the application's configuration, infrastructure,
// repositories, services, HTTP handlers, and server startup.
package app

import (
	"context"
	"fmt"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	logg "github.com/akhmed9505/comment-tree/internal/logger"
	repocomments "github.com/akhmed9505/comment-tree/internal/repository/comments"
	svccomments "github.com/akhmed9505/comment-tree/internal/service/comments"
	"github.com/akhmed9505/comment-tree/internal/transport"
	"github.com/akhmed9505/comment-tree/internal/transport/http/handler/comments"

	"github.com/wb-go/wbf/logger"
)

type App struct {
	Config       *config.Config
	Logger       *logger.ZerologAdapter
	Repositories *Repositories
	Services     *Services
	Server       *transport.Server
}

type Repositories struct {
	Comments *repocomments.Repository
}

type Services struct {
	Comments *svccomments.Service
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load("./configs/config.yml")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Create logger.
	log := logg.New(cfg)

	// Create PostgreSQL pool.
	pgPool, err := postgres.New(cfg.Postgres.ConnectionURL, log)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	repos := &Repositories{
		Comments: repocomments.New(pgPool),
	}

	svcs := &Services{
		Comments: svccomments.New(log, repos.Comments),
	}

	handlers := transport.Handlers{
		Comments: comments.New(svcs.Comments, cfg, log),
	}

	router := transport.NewRouter(handlers)
	server := transport.New(cfg, log, router)

	return &App{
		Config:       cfg,
		Logger:       log,
		Repositories: repos,
		Services:     svcs,
		Server:       server,
	}, nil
}

// Run starts the HTTP server and handles graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("starting server on %s", a.Config.HTTP.Port)
	return a.Server.Run(ctx)
}
