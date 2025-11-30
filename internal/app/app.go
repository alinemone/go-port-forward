// Package app coordinates the application components.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/alinemone/go-port-forward/internal/config"
	"github.com/alinemone/go-port-forward/internal/logger"
	"github.com/alinemone/go-port-forward/internal/service"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// App coordinates all application components.
type App struct {
	config  *config.Config
	logger  *logger.Logger
	storage *storage.Storage
	manager *service.Manager
}

// New creates a new application instance.
func New() (*App, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create logger
	log, err := logger.New(cfg.LogMaxSize, cfg.LogMaxBackups, logger.LevelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create storage
	stor := storage.New()

	// Create service manager
	mgr := service.NewManager(stor, log, cfg)

	return &App{
		config:  cfg,
		logger:  log,
		storage: stor,
		manager: mgr,
	}, nil
}

// Run starts the TUI and runs the specified services.
func (a *App) Run(ctx context.Context, serviceNames []string) error {
	a.logger.Info("Starting application with services: %v", serviceNames)

	// Start all requested services with a small delay between each
	// This prevents kubectl lock conflicts on ~/.kube/config
	for i, name := range serviceNames {
		if err := a.manager.Start(ctx, name); err != nil {
			a.logger.Error("Failed to start service %q: %v", name, err)
			return fmt.Errorf("failed to start service %q: %w", name, err)
		}

		// Add delay between service starts (except after the last one)
		if i < len(serviceNames)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Create UI model
	model := ui.New(a.manager, a.config)

	// Start Bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		a.logger.Error("TUI error: %v", err)
		return fmt.Errorf("TUI error: %w", err)
	}

	a.logger.Info("Application stopped")
	return nil
}

// Close cleans up application resources.
func (a *App) Close() error {
	a.manager.StopAll()
	return a.logger.Close()
}

// GetStorage returns the storage instance.
func (a *App) GetStorage() *storage.Storage {
	return a.storage
}

// GetLogger returns the logger instance.
func (a *App) GetLogger() *logger.Logger {
	return a.logger
}
