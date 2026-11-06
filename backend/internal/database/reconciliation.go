package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"voltafinbook/backend/migrations"
)

// SetupReconciliation adds only the independently named statement tables on
// legacy installations. Databases with migration history use the normal runner.
func SetupReconciliation(ctx context.Context, db *sql.DB, apply bool) (bool, error) {
	var history bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations')`).Scan(&history); err != nil {
		return false, err
	}
	if history && apply {
		_, err := ApplyMigrations(ctx, db)
		return err == nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if apply {
		var locked sql.NullInt64
		if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK('volta_finbook_mysql_migrations',10)`).Scan(&locked); err != nil {
			return false, err
		}
		if !locked.Valid || locked.Int64 != 1 {
			return false, errors.New("could not acquire statement setup lock")
		}
		defer func() {
			release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var value sql.NullInt64
			_ = conn.QueryRowContext(release, `SELECT RELEASE_LOCK('volta_finbook_mysql_migrations')`).Scan(&value)
		}()
	}
	all, err := migrations.MySQLAll()
	if err != nil {
		return false, err
	}
	names := []string{"reconciliation_statements", "reconciliation_statement_rows"}
	checks := []string{
		`SELECT id,file_name,file_sha256,statement_kind,source_bytes,row_count,uploaded_by,created_at FROM reconciliation_statements LIMIT 0`,
		`SELECT id,statement_id,line_number,source_section,source_row,cells FROM reconciliation_statement_rows LIMIT 0`,
	}
	for _, migration := range all {
		if migration.Version != 8 {
			continue
		}
		statements, err := splitMySQLStatements(migration.SQL)
		if err != nil {
			return false, err
		}
		if len(statements) != len(names) {
			return false, errors.New("unexpected reconciliation setup statements")
		}
		ready := true
		for i, name := range names {
			var exists bool
			if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?)`, name).Scan(&exists); err != nil {
				return false, err
			}
			if !exists {
				if !apply {
					ready = false
					continue
				}
				if _, err = conn.ExecContext(ctx, statements[i]); err != nil {
					return false, err
				}
			}
			if _, err = conn.ExecContext(ctx, checks[i]); err != nil {
				return false, err
			}
		}
		return ready, nil
	}
	return false, errors.New("statement migration unavailable")
}
