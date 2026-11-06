package mir

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
	"voltafinbook/backend/internal/database"
)

func TestLegacyMIRSetupPreservesExistingTables(t *testing.T) {
	if os.Getenv("FINBOOK_MIR_INTEGRATION") != "1" {
		t.Skip("requires isolated MySQL integration opt-in")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINBOOK_ROOT", root)
	config, err := database.LoadConfig()
	if err != nil {
		t.Fatal("test configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := database.Connect(ctx, config)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer admin.Close()
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "finbook_mir_setup_test_" + hex.EncodeToString(suffix[:])
	if !regexp.MustCompile(`^finbook_mir_setup_test_[0-9a-f]{16}$`).MatchString(name) {
		t.Fatal("unsafe test schema")
	}
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, release := context.WithTimeout(context.Background(), 15*time.Second)
		defer release()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error("isolated schema cleanup failed")
		}
	}()
	isolated := config.Clone()
	isolated.DBName = name
	db, err := database.Connect(ctx, isolated)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE users(id BINARY(16) PRIMARY KEY)`)
	exec(`CREATE TABLE mir_files(id INT PRIMARY KEY,legacy_value INT NOT NULL)`)
	actor := bytes.Repeat([]byte{1}, 16)
	exec(`INSERT INTO users(id) VALUES(?)`, actor)
	exec(`INSERT INTO mir_files VALUES(1,314159)`)
	// Partial staging tables are rejected before any further DDL is executed.
	exec(`CREATE TABLE mir_import_batch(id INT PRIMARY KEY)`)
	if _, err := database.ApplyMIRSchema(ctx, db); err == nil {
		t.Fatal("partial staging schema should require inspection")
	}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()`).Scan(&tables); err != nil || tables != 3 {
		t.Fatal("partial setup changed existing schema")
	}
	exec(`DROP TABLE mir_import_batch`)
	status, err := database.ApplyMIRSchema(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || len(status.Missing) != 0 || len(status.Existing) < 20 {
		t.Fatal("MIR tables were not installed")
	}
	id, err := (Repository{db}).StartBatch(ctx, actor)
	if err != nil || id < 1 {
		t.Fatalf("batch creation after legacy setup: %v", err)
	}
	var value, users, history int
	if err := db.QueryRowContext(ctx, `SELECT legacy_value FROM mir_files WHERE id=1`).Scan(&value); err != nil || value != 314159 {
		t.Fatal("legacy data changed")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil || users != 1 {
		t.Fatal("user data changed")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations'`).Scan(&history); err != nil || history != 0 {
		t.Fatal("legacy setup invented foundation migration history")
	}
	if _, err = database.ApplyMIRSchema(ctx, db); err != nil {
		t.Fatal("repeated setup should be harmless", err)
	}
	var batches int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_import_batch`).Scan(&batches); err != nil || batches != 1 {
		t.Fatal("repeated setup changed import batches")
	}
	ready, err := database.SetupSalesTypes(ctx, db, true)
	if err != nil || !ready {
		t.Fatalf("sales type setup: %v", err)
	}
	var types int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sales_types WHERE active=TRUE").Scan(&types); err != nil || types != 3 {
		t.Fatal("sales type defaults missing")
	}
	exec("UPDATE sales_types SET name='Edited label' WHERE code='DOM'")
	if _, err := database.SetupSalesTypes(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	var edited string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sales_types WHERE code='DOM'").Scan(&edited); err != nil || edited != "Edited label" {
		t.Fatal("setup replaced configured sales type")
	}
	t.Log("Legacy MIR setup retained existing data, installed staging tables, created a batch, and rejected partial schemas")
}
