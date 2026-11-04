package database

import (
	"context"
	"database/sql"
	"errors"

	"voltafinbook/backend/migrations"
)

type MigrationStatus struct {
	Applied int  `json:"applied"`
	Pending int  `json:"pending"`
	Current bool `json:"current"`
}

func CheckMigrations(ctx context.Context, db *sql.DB) (MigrationStatus, error) {
	if db == nil {
		return MigrationStatus{}, errors.New("database not configured")
	}
	available, err := migrations.MySQLAll()
	if err != nil {
		return MigrationStatus{}, err
	}
	var exists bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'schema_migrations'
	)`).Scan(&exists)
	if err != nil {
		return MigrationStatus{}, err
	}
	if !exists {
		var tables int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`).Scan(&tables); err != nil {
			return MigrationStatus{}, err
		}
		if tables != 0 {
			return MigrationStatus{}, errors.New("application tables exist without recognized migration history")
		}
		return MigrationStatus{Pending: len(available)}, nil
	}
	var applied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return MigrationStatus{}, err
	}
	if applied != 0 {
		return MigrationStatus{}, errors.New("MySQL migration history is not recognized")
	}
	return MigrationStatus{Pending: len(available)}, nil
}

func ApplyMigrations(context.Context, *sql.DB) (MigrationStatus, error) {
	return MigrationStatus{}, errors.New("MySQL migrations are not available yet; PostgreSQL SQL was not executed")
}
