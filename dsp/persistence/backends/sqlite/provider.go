package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/extra/bundebug"
	"github.com/uptrace/bun/migrate"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/migrations"
)

// Provider is a StorageProvider instance for the sqlite backend.
type Provider struct {
	h           *bun.DB
	s           *sql.DB
	lockTimeout int
}

// New returns a provider instance, or an error if it failed to create/open a database.
func New(ctx context.Context, inMemory, debug bool, dbPath string) (*Provider, error) {
	if inMemory {
		dbPath = "file::memory:?cache=shared"
	}
	sqlDB, err := sql.Open(sqliteshim.DriverName(), dbPath)
	if err != nil {
		return nil, err
	}

	db := bun.NewDB(sqlDB, sqlitedialect.New())
	if debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}
	return &Provider{h: db, s: sqlDB, lockTimeout: 60}, nil
}

// Sets the lock timeout, only intended for tests.
func (p *Provider) SetLockTimeout(secs int) {
	p.lockTimeout = secs
}

// Migrate runs the latest migrations.
func (p *Provider) Migrate(ctx context.Context) error {
	migrator := migrate.NewMigrator(p.h, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		return err
	}

	if err := migrator.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		if err := migrator.Unlock(ctx); err != nil {
			panic(fmt.Sprintf("Could not unlock migrator and database: %s", err))
		}
	}()

	group, err := migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	if group.IsZero() {
		ctxslog.Info(ctx, "No migrations to perform")
		return nil
	}

	ctxslog.Info(ctx, "Migration complete", "group", group.String())
	return nil
}

// Close closes the handle.
func (p *Provider) Close() error {
	return p.s.Close()
}
