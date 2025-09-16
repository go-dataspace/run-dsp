package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Provider is a StorageProvider instance for the sqlite backend.
type Provider struct {
	handle *sql.DB
}

//go:embed migrations/*.sql
var migrations embed.FS

// New returns a provider instance, or an error if it failed to create/open a database.
func New(ctx context.Context, inMemory bool, dbPath string) (*Provider, error) {
	dbPath = "file:" + dbPath
	if inMemory {
		dbPath = "file:memfile.db?cache=shared&mode=memory"
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("couldn't ping database: %w", err)
	}
	return &Provider{handle: db}, nil
}

// Migrate runs the latest migrations.
func (p *Provider) Migrate() error {
	d, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("couldn't initialise iofs: %w", err)
	}
	s, err := sqlite3.WithInstance(p.handle, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("couldn't initialise sqlite migration instance: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", d, "sqlite", s)
	if err != nil {
		return fmt.Errorf("couldn't initalise migration: %w", err)
	}
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Close closes the handle.
func (p *Provider) Close() error {
	return p.handle.Close()
}
