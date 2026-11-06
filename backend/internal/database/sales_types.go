package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"voltafinbook/backend/migrations"
)

// SetupSalesTypes also supports the existing installation without foundation
// migration history. It adds only the new master and never reseeds edited types.
func SetupSalesTypes(ctx context.Context, db *sql.DB, apply bool) (bool, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='sales_types')`).Scan(&exists); err != nil {
		return false, err
	}
	if !apply {
		return exists, nil
	}
	var history bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations')`).Scan(&history); err != nil {
		return false, err
	}
	if history {
		_, err := ApplyMigrations(ctx, db)
		return err == nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var locked sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK('volta_finbook_mysql_migrations',10)`).Scan(&locked); err != nil {
		return false, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		return false, errors.New("could not acquire sales type setup lock")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var result sql.NullInt64
		_ = conn.QueryRowContext(release, `SELECT RELEASE_LOCK('volta_finbook_mysql_migrations')`).Scan(&result)
	}()
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='sales_types')`).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		_, err = conn.ExecContext(ctx, `SELECT id,code,name,sales_class,active,deleted_at FROM sales_types LIMIT 0`)
		return err == nil, err
	}
	all, err := migrations.MySQLAll()
	if err != nil {
		return false, err
	}
	for _, migration := range all {
		if migration.Version == 7 {
			statements, err := splitMySQLStatements(migration.SQL)
			if err != nil {
				return false, err
			}
			for _, statement := range statements {
				if _, err = conn.ExecContext(ctx, statement); err != nil {
					return false, err
				}
			}
			return true, nil
		}
	}
	return false, errors.New("sales type migration unavailable")
}
