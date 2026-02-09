// Package database manages the PostgreSQL connection pool, health checks, and
// database migrations for AmityVox. It uses pgx for direct PostgreSQL access
// without an ORM, and golang-migrate for schema migrations.
package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations
var migrationsFS embed.FS

// DB wraps a pgx connection pool and provides health checks and graceful shutdown.
type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

// New creates a new database connection pool with the given PostgreSQL URL and
// maximum connection count. It verifies connectivity with a ping before returning.
func New(ctx context.Context, databaseURL string, maxConns int, logger *slog.Logger) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	config.MaxConns = int32(maxConns)
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Info("database connection established",
		slog.String("host", config.ConnConfig.Host),
		slog.Int("max_conns", maxConns),
	)

	return &DB{Pool: pool, logger: logger}, nil
}

// HealthCheck verifies the database connection is alive by executing a simple query.
func (db *DB) HealthCheck(ctx context.Context) error {
	var result int
	err := db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database health check: %w", err)
	}
	return nil
}

// Close gracefully shuts down the connection pool.
func (db *DB) Close() {
	db.logger.Info("closing database connection pool")
	db.Pool.Close()
}

// MigrateUp runs all pending database migrations from the embedded migrations
// directory. It returns the number of applied migrations or an error.
func MigrateUp(databaseURL string, logger *slog.Logger) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}

	logger.Info("running database migrations (up)")

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations up: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("getting migration version: %w", err)
	}

	logger.Info("migrations complete",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return fmt.Errorf("closing migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("closing migration database: %w", dbErr)
	}

	return nil
}

// MigrateDown rolls back all database migrations. Use with caution.
func MigrateDown(databaseURL string, logger *slog.Logger) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}

	logger.Warn("running database migrations (down) — this will drop all tables")

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations down: %w", err)
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return fmt.Errorf("closing migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("closing migration database: %w", dbErr)
	}

	logger.Info("migrations rolled back")
	return nil
}

// MigrateStatus returns the current migration version and dirty state.
func MigrateStatus(databaseURL string) (version uint, dirty bool, err error) {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return 0, false, err
	}

	version, dirty, err = m.Version()
	if err != nil && err != migrate.ErrNoChange {
		return 0, false, fmt.Errorf("getting migration status: %w", err)
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return version, dirty, fmt.Errorf("closing migration source: %w", srcErr)
	}
	if dbErr != nil {
		return version, dirty, fmt.Errorf("closing migration database: %w", dbErr)
	}

	return version, dirty, nil
}

// newMigrator creates a new migrate.Migrate instance using the embedded SQL files.
func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating migrator: %w", err)
	}

	return m, nil
}
