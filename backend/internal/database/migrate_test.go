package database

import (
	"context"
	"strings"
	"testing"
)

func TestMySQLMigrationApplyFailsUntilSchemaPortIsAvailable(t *testing.T) {
	_, err := ApplyMigrations(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "MySQL migrations are not available") {
		t.Fatal("migration runner must refuse to apply PostgreSQL-only SQL to MySQL")
	}
}

func TestMigrationStatusRequiresDatabase(t *testing.T) {
	_, err := CheckMigrations(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "database not configured") {
		t.Fatal("migration status must report a missing database")
	}
}
