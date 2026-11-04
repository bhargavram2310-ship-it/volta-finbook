package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"voltafinbook/backend/internal/database"
)

func TestNewUUIDFormatsRandomRFC4122Identifier(t *testing.T) {
	first, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 36 || first[14] != '4' || !strings.Contains(first, "-") {
		t.Fatal("administrator ID must be a unique version 4 UUID")
	}
}

func TestEqualPassword(t *testing.T) {
	if !equalPassword([]byte("same"), []byte("same")) || equalPassword([]byte("same"), []byte("other")) || equalPassword([]byte("short"), []byte("longer")) {
		t.Fatal("password confirmation comparison is incorrect")
	}
}

func TestDefaultAdministratorCredentials(t *testing.T) {
	if defaultUsername != "SuperAdmin" || defaultPassword != defaultUsername {
		t.Fatal("default administrator username and password must both be SuperAdmin")
	}
}

func TestUUIDBytes(t *testing.T) {
	value := "00112233-4455-4677-8899-aabbccddeeff"
	raw, err := uuidBytes(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}) {
		t.Fatalf("UUID was not encoded in canonical byte order: %x", raw)
	}
	if _, err := uuidBytes("not-a-uuid"); err == nil {
		t.Fatal("invalid UUID was accepted")
	}
}

func TestMySQLBootstrapSchemaIntegration(t *testing.T) {
	if os.Getenv("FINBOOK_MYSQL_INTEGRATION") != "1" {
		t.Skip("set FINBOOK_MYSQL_INTEGRATION=1 to run against the configured MySQL database")
	}
	config, err := database.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	employeeID, userID, roleID, permissionID, sessionID := make([]byte, 16), make([]byte, 16), make([]byte, 16), make([]byte, 16), make([]byte, 16)
	for _, id := range [][]byte{employeeID, userID, roleID, permissionID, sessionID} {
		if _, err := rand.Read(id); err != nil {
			t.Fatal(err)
		}
	}
	suffix := strings.ReplaceAll(testUUID(t), "-", "")
	employeeCode := "schema-test-" + suffix
	username := "schema-test-" + suffix
	email := "schema-test-" + suffix + "@example.invalid"

	if _, err := tx.ExecContext(ctx, `INSERT INTO employees
		(id,employee_code,name,official_email,employment_status,active,version)
		VALUES(?,?,?,?,'Active',TRUE,1)`, employeeID, employeeCode, "Schema Test", email); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users
		(id,employee_id,username,display_name,login_email,password_hash,password_reset_required,account_status,active,version)
		VALUES(?,?,?,?,?,?,FALSE,'Active',TRUE,1)`, userID, employeeID, username, "Schema Test", email, "integration-placeholder-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET user_id=? WHERE id=?`, userID, employeeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO roles
		(id,code,name,description,active,created_by,updated_by,version)
		VALUES(?,?,?,'integration test',TRUE,?,?,1)`, roleID, "SCHEMA_TEST_"+suffix, "Schema Test", userID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO permissions(id,module,screen,action)
		VALUES(?,?,?,?)`, permissionID, "schema_test_"+suffix, "schema_test_"+suffix, "schema_test"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO role_permissions(role_id,permission_id,created_by)
		VALUES(?,?,?)`, roleID, permissionID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id,created_by)
		VALUES(?,?,?)`, userID, roleID, userID); err != nil {
		t.Fatal(err)
	}
	tokenHash := make([]byte, 32)
	if _, err := rand.Read(tokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at)
		VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 12 HOUR))`, sessionID, userID, tokenHash); err != nil {
		t.Fatal(err)
	}
}

func testUUID(t *testing.T) string {
	t.Helper()
	value, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
