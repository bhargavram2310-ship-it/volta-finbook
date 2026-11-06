package database

import (
	"context"
	"strings"
	"testing"

	"voltafinbook/backend/migrations"
)

func TestMySQLMigrationApplyRequiresDatabase(t *testing.T) {
	_, err := ApplyMigrations(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "database not configured") {
		t.Fatal("migration runner must report a missing database")
	}
}

func TestMigrationStatusRequiresDatabase(t *testing.T) {
	_, err := CheckMigrations(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "database not configured") {
		t.Fatal("migration status must report a missing database")
	}
}

func TestSplitMySQLStatements(t *testing.T) {
	script := "-- comment with ;\n" +
		"CREATE TABLE items (value VARCHAR(40) DEFAULT 'left;right');\n" +
		"INSERT INTO items VALUES (\"semi;colon\"); # trailing ;\n" +
		"/* block ; comment */ UPDATE items SET value='it''s; fine';"
	statements, err := splitMySQLStatements(script)
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 3 {
		t.Fatalf("got %d statements, want 3: %#v", len(statements), statements)
	}
}

func TestSplitMySQLStatementsRejectsUnterminatedLiteral(t *testing.T) {
	if _, err := splitMySQLStatements("SELECT 'unfinished"); err == nil {
		t.Fatal("unterminated literals must be rejected")
	}
}

func TestMySQLMigrationScriptsCanBeSplitIntoStatements(t *testing.T) {
	all, err := migrations.MySQLAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range all {
		statements, err := splitMySQLStatements(migration.SQL)
		if err != nil {
			t.Errorf("%s: %v", migration.Name, err)
		} else if len(statements) == 0 {
			t.Errorf("%s has no SQL statements", migration.Name)
		}
	}
}
