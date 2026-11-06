package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
	"voltafinbook/backend/migrations"
)

type MIRSchemaStatus struct {
	Ready    bool     `json:"ready"`
	Existing []string `json:"existing_tables"`
	Missing  []string `json:"missing_tables"`
	Checksum string   `json:"checksum"`
}

// Legacy installations may have application tables without migration history.
// This additive setup installs only the independently named MIR staging tables;
// it never invents foundation migration history or alters existing tables.
func mirSchemaStatements() ([]string, []string, string, error) {
	all, err := migrations.MySQLAll()
	if err != nil {
		return nil, nil, "", err
	}
	pattern := regexp.MustCompile("^CREATE TABLE (mir_[a-z_]+)\\s*\\(")
	for _, migration := range all {
		if migration.Version != 6 {
			continue
		}
		statements, err := splitMySQLStatements(migration.SQL)
		if err != nil {
			return nil, nil, "", err
		}
		names := []string{}
		for _, statement := range statements {
			match := pattern.FindStringSubmatch(statement)
			if len(match) != 2 {
				return nil, nil, "", errors.New("MIR setup contains a statement other than an additive MIR table creation")
			}
			names = append(names, match[1])
		}
		if len(names) == 0 {
			return nil, nil, "", errors.New("MIR staging migration has no tables")
		}
		return statements, names, migration.Checksum, nil
	}
	return nil, nil, "", errors.New("MIR staging migration unavailable")
}

func MIRSchemaPlan(ctx context.Context, db *sql.DB) (MIRSchemaStatus, error) {
	_, names, checksum, err := mirSchemaStatements()
	if err != nil {
		return MIRSchemaStatus{}, err
	}
	status := MIRSchemaStatus{Existing: []string{}, Missing: []string{}, Checksum: checksum}
	for _, name := range names {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?)`, name).Scan(&exists); err != nil {
			return status, err
		}
		if exists {
			status.Existing = append(status.Existing, name)
		} else {
			status.Missing = append(status.Missing, name)
		}
	}
	sort.Strings(status.Existing)
	sort.Strings(status.Missing)
	status.Ready = len(status.Missing) == 0
	return status, nil
}

func ApplyMIRSchema(ctx context.Context, db *sql.DB) (MIRSchemaStatus, error) {
	var history bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations')`).Scan(&history); err != nil {
		return MIRSchemaStatus{}, err
	}
	if history {
		if _, err := ApplyMigrations(ctx, db); err != nil {
			return MIRSchemaStatus{}, err
		}
		return MIRSchemaPlan(ctx, db)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return MIRSchemaStatus{}, err
	}
	defer conn.Close()
	var locked sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK('volta_finbook_mysql_migrations',10)`).Scan(&locked); err != nil {
		return MIRSchemaStatus{}, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		return MIRSchemaStatus{}, errors.New("could not acquire MIR schema setup lock")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(release, `SELECT RELEASE_LOCK('volta_finbook_mysql_migrations')`).Scan(&released)
	}()
	plan, err := MIRSchemaPlan(ctx, db)
	if err != nil {
		return plan, err
	}
	if plan.Ready {
		return plan, nil
	}
	if len(plan.Existing) > 0 {
		return plan, errors.New("partial MIR staging schema found; inspect existing tables before continuing")
	}
	var userIDType string
	if err = conn.QueryRowContext(ctx, `SELECT column_type FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='id'`).Scan(&userIDType); err != nil || userIDType != "binary(16)" {
		return plan, errors.New("existing users.id must be BINARY(16) before MIR staging can be installed")
	}
	statements, _, _, err := mirSchemaStatements()
	if err != nil {
		return plan, err
	}
	for i, statement := range statements {
		if _, err = conn.ExecContext(ctx, statement); err != nil {
			return plan, fmt.Errorf("MIR setup stopped at table %d; inspect the partial schema before retrying: %w", i+1, err)
		}
	}
	return MIRSchemaPlan(ctx, db)
}
