package backend

import (
	"context"

	"charm.land/log/v2"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/db"
	"github.com/wyrd-company/gelato/pkg/store"
	"github.com/wyrd-company/gelato/pkg/task"
)

// Backend is the Gelato backend that handles users, repositories, and
// server settings management and operations.
type Backend struct {
	ctx     context.Context
	cfg     *config.Config
	db      *db.DB
	store   store.Store
	logger  *log.Logger
	cache   *cache
	manager *task.Manager
}

// New returns a new Gelato backend.
func New(ctx context.Context, cfg *config.Config, db *db.DB, st store.Store) *Backend {
	logger := log.FromContext(ctx).WithPrefix("backend")
	b := &Backend{
		ctx:     ctx,
		cfg:     cfg,
		db:      db,
		store:   st,
		logger:  logger,
		manager: task.NewManager(ctx),
	}

	// TODO: implement a proper caching interface
	cache := newCache(b, 1000)
	b.cache = cache

	return b
}
