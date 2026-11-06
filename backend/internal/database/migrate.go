package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"voltafinbook/backend/migrations"
)

type MigrationStatus struct {
	Applied int  `json:"applied"`
	Pending int  `json:"pending"`
	Current bool `json:"current"`
}

type migrationQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func CheckMigrations(ctx context.Context, db *sql.DB) (MigrationStatus, error) {
	if db == nil {
		return MigrationStatus{}, errors.New("database not configured")
	}
	return checkMigrations(ctx, db)
}

func checkMigrations(ctx context.Context, db migrationQueryer) (MigrationStatus, error) {
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
		return MigrationStatus{}, fmt.Errorf("check MySQL migration table: %w", err)
	}
	if !exists {
		var tables int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE()`).Scan(&tables); err != nil {
			return MigrationStatus{}, fmt.Errorf("inspect MySQL schema before migration: %w", err)
		}
		if tables != 0 {
			return MigrationStatus{}, errors.New("application tables exist without recognized migration history")
		}
		return MigrationStatus{Pending: len(available)}, nil
	}

	rows, err := db.QueryContext(ctx, `SELECT version, name, checksum, state
		FROM schema_migrations ORDER BY version`)
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("read MySQL migration history: %w", err)
	}
	defer rows.Close()
	applied := 0
	for rows.Next() {
		var version int
		var name, checksum, state string
		if err := rows.Scan(&version, &name, &checksum, &state); err != nil {
			return MigrationStatus{}, fmt.Errorf("read MySQL migration history: %w", err)
		}
		if state != "applied" {
			return MigrationStatus{}, fmt.Errorf("MySQL migration %d is incomplete; manual recovery is required before migrations can continue", version)
		}
		if applied >= len(available) {
			return MigrationStatus{}, fmt.Errorf("unknown MySQL migration version %d", version)
		}
		expected := available[applied]
		if version != expected.Version || name != expected.Name || checksum != expected.Checksum {
			return MigrationStatus{}, fmt.Errorf("MySQL migration history differs from the available migration sequence at version %d", version)
		}
		applied++
	}
	if err := rows.Err(); err != nil {
		return MigrationStatus{}, fmt.Errorf("read MySQL migration history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return MigrationStatus{}, fmt.Errorf("close MySQL migration history: %w", err)
	}
	if applied == 0 {
		var tables int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name <> 'schema_migrations'`).Scan(&tables); err != nil {
			return MigrationStatus{}, fmt.Errorf("inspect MySQL schema before migration: %w", err)
		}
		if tables != 0 {
			return MigrationStatus{}, errors.New("application tables exist without recognized migration history")
		}
	}
	pending := len(available) - applied
	return MigrationStatus{Applied: applied, Pending: pending, Current: pending == 0}, nil
}

func ApplyMigrations(ctx context.Context, db *sql.DB) (status MigrationStatus, err error) {
	if db == nil {
		return MigrationStatus{}, errors.New("database not configured")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("open MySQL migration connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close MySQL migration connection: %w", closeErr)
		}
	}()

	var locked sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK('volta_finbook_mysql_migrations', 10)`).Scan(&locked); err != nil {
		return MigrationStatus{}, fmt.Errorf("acquire MySQL migration lock: %w", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		return MigrationStatus{}, errors.New("could not acquire MySQL migration lock")
	}
	defer func() {
		var released sql.NullInt64
		if releaseErr := conn.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK('volta_finbook_mysql_migrations')`).Scan(&released); releaseErr != nil && err == nil {
			err = fmt.Errorf("release MySQL migration lock: %w", releaseErr)
		} else if releaseErr == nil && (!released.Valid || released.Int64 != 1) && err == nil {
			err = errors.New("MySQL migration lock was not released")
		}
	}()

	status, err = checkMigrations(ctx, conn)
	if err != nil {
		return MigrationStatus{}, err
	}
	if status.Current {
		return status, nil
	}
	var historyExists bool
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'schema_migrations'
	)`).Scan(&historyExists); err != nil {
		return MigrationStatus{}, fmt.Errorf("check MySQL migration table: %w", err)
	}
	if !historyExists {
		if _, err = conn.ExecContext(ctx, `CREATE TABLE schema_migrations (
			version INT NOT NULL,
			name VARCHAR(191) NOT NULL,
			checksum CHAR(64) NOT NULL,
			state VARCHAR(16) NOT NULL,
			started_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			applied_at DATETIME(6) NULL,
			PRIMARY KEY (version),
			CONSTRAINT schema_migrations_state CHECK (state IN ('running','applied'))
		) ENGINE=InnoDB`); err != nil {
			return MigrationStatus{}, fmt.Errorf("create MySQL migration history: %w", err)
		}
	}
	available, err := migrations.MySQLAll()
	if err != nil {
		return MigrationStatus{}, err
	}
	for _, migration := range available[status.Applied:] {
		if _, err = conn.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum,state)
			VALUES(?,?,?,'running')`, migration.Version, migration.Name, migration.Checksum); err != nil {
			return MigrationStatus{}, fmt.Errorf("record MySQL migration %d start: %w", migration.Version, err)
		}
		statements, splitErr := splitMySQLStatements(migration.SQL)
		if splitErr != nil {
			return MigrationStatus{}, fmt.Errorf("parse MySQL migration %d: %w", migration.Version, splitErr)
		}
		for i, statement := range statements {
			if _, err = conn.ExecContext(ctx, statement); err != nil {
				return MigrationStatus{}, fmt.Errorf("execute MySQL migration %d statement %d: %w", migration.Version, i+1, err)
			}
		}
		if _, err = conn.ExecContext(ctx, `UPDATE schema_migrations
			SET state='applied', applied_at=CURRENT_TIMESTAMP(6) WHERE version=? AND state='running'`, migration.Version); err != nil {
			return MigrationStatus{}, fmt.Errorf("mark MySQL migration %d applied: %w", migration.Version, err)
		}
	}
	return checkMigrations(ctx, conn)
}

func splitMySQLStatements(script string) ([]string, error) {
	var statements []string
	var current strings.Builder
	const (
		normal = iota
		singleQuoted
		doubleQuoted
		backtickQuoted
		lineComment
		blockComment
	)
	state := normal
	for i := 0; i < len(script); i++ {
		ch := script[i]
		next := byte(0)
		if i+1 < len(script) {
			next = script[i+1]
		}
		switch state {
		case normal:
			switch {
			case ch == '\'':
				state = singleQuoted
				current.WriteByte(ch)
			case ch == '"':
				state = doubleQuoted
				current.WriteByte(ch)
			case ch == '`':
				state = backtickQuoted
				current.WriteByte(ch)
			case ch == '#' || (ch == '-' && next == '-' && (i+2 == len(script) || script[i+2] <= ' ')):
				state = lineComment
				if ch == '-' {
					i++
				}
			case ch == '/' && next == '*':
				state = blockComment
				i++
			case ch == ';':
				if statement := strings.TrimSpace(current.String()); statement != "" {
					statements = append(statements, statement)
				}
				current.Reset()
			default:
				current.WriteByte(ch)
			}
		case singleQuoted, doubleQuoted, backtickQuoted:
			current.WriteByte(ch)
			quote := byte('\'')
			if state == doubleQuoted {
				quote = '"'
			} else if state == backtickQuoted {
				quote = '`'
			}
			if ch == '\\' && state != backtickQuoted && i+1 < len(script) {
				i++
				current.WriteByte(script[i])
			} else if ch == quote {
				if next == quote {
					i++
					current.WriteByte(next)
				} else {
					state = normal
				}
			}
		case lineComment:
			if ch == '\n' {
				state = normal
				current.WriteByte(ch)
			}
		case blockComment:
			if ch == '*' && next == '/' {
				state = normal
				i++
				current.WriteByte(' ')
			}
		}
	}
	if state == singleQuoted || state == doubleQuoted || state == backtickQuoted || state == blockComment {
		return nil, errors.New("unterminated quoted value or block comment")
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}
