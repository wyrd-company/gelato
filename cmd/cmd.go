package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"
	"github.com/wyrd-company/gelato/pkg/backend"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/db"
	"github.com/wyrd-company/gelato/pkg/events"
	"github.com/wyrd-company/gelato/pkg/hooks"
	"github.com/wyrd-company/gelato/pkg/store"
	"github.com/wyrd-company/gelato/pkg/store/database"
)

// InitBackendContext initializes the backend context.
func InitBackendContext(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	cfg := config.FromContext(ctx)
	if _, err := os.Stat(cfg.DataPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(cfg.DataPath, os.ModePerm); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	}
	dbx, err := db.Open(ctx, cfg.DB.Driver, cfg.DB.DataSource)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	ctx = db.WithContext(ctx, dbx)
	dbstore := database.New(ctx, dbx)
	ctx = store.WithContext(ctx, dbstore)
	publisher, err := events.NewNATSPublisher(cfg)
	if err != nil {
		return err
	}
	ctx = events.WithPublisher(ctx, publisher)
	be := backend.New(ctx, cfg, dbx, dbstore)
	ctx = backend.WithContext(ctx, be)

	cmd.SetContext(ctx)

	return nil
}

// CloseDBContext closes the database context.
func CloseDBContext(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	dbx := db.FromContext(ctx)
	if dbx != nil {
		if err := dbx.Close(); err != nil {
			return fmt.Errorf("close database: %w", err)
		}
	}
	if err := events.FromContext(ctx).Close(); err != nil {
		return fmt.Errorf("close event publisher: %w", err)
	}

	return nil
}

// InitializeHooks initializes the hooks.
func InitializeHooks(ctx context.Context, cfg *config.Config, be *backend.Backend) error {
	repos, err := be.Repositories(ctx)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if err := hooks.GenerateHooks(ctx, cfg, repo.Name()); err != nil {
			return err
		}
	}

	return nil
}
