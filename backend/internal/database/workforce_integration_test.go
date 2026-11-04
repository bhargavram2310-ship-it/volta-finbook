//go:build postgres_legacy

// These PostgreSQL integration scenarios are retained for reference and are
// excluded from the MySQL backend test suite until ported.
package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"voltafinbook/backend/migrations"
)

// Explicit cloud opt-in. Every schema and fixture is rolled back, never committed.
func TestPostgreSQLWorkforceIntegrity(t *testing.T) {
	if os.Getenv("FINBOOK_DB_INTEGRATION") != "1" {
		t.Skip("requires explicit cloud integration opt-in")
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("database configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	conn, err := Connect(ctx, config)
	if err != nil {
		t.Fatal("cloud database connection failed; credentials suppressed")
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal("cannot start rollback-only transaction")
	}
	defer tx.Rollback(context.Background())
	schema := fmt.Sprintf("finbook_test_workforce_%d", time.Now().UnixNano())
	sql := func(s string) string { return strings.ReplaceAll(s, "finbook", schema) }
	exec := func(s string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(ctx, sql(s), args...); e != nil {
			var p *pgconn.PgError
			if errors.As(e, &p) {
				t.Fatalf("integrity operation failed (SQLSTATE %s)", p.Code)
			}
			t.Fatal("integrity operation failed")
		}
	}
	id := func(s string, args ...any) string {
		t.Helper()
		var value string
		if e := tx.QueryRow(ctx, sql(s), args...).Scan(&value); e != nil {
			t.Fatal("fixture identity creation failed")
		}
		return value
	}
	reject := func(code, s string, args ...any) {
		t.Helper()
		savepoint, e := tx.Begin(ctx)
		if e != nil {
			t.Fatal("savepoint failed")
		}
		_, e = savepoint.Exec(ctx, sql(s), args...)
		_ = savepoint.Rollback(ctx)
		var p *pgconn.PgError
		if !errors.As(e, &p) || p.Code != code {
			t.Fatalf("expected rejection SQLSTATE %s", code)
		}
	}
	expect := func(s string, args ...any) {
		t.Helper()
		var ok bool
		if e := tx.QueryRow(ctx, sql(s), args...).Scan(&ok); e != nil || !ok {
			t.Fatal("workforce integrity assertion failed")
		}
	}
	exec(bootstrapSQL)
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		exec(m.SQL)
	}

	adminEmployee := id("INSERT INTO finbook.employees(employee_code,name,official_email) VALUES('ADMIN','Test administrator','admin@example.test') RETURNING id")
	admin := id("INSERT INTO finbook.users(employee_id,username,display_name,login_email,password_hash,password_reset_required) VALUES($1,'admin','Test administrator','admin@example.test','rollback-only-test-hash',false) RETURNING id", adminEmployee)
	exec("SELECT set_config('finbook.actor_id',$1,true),set_config('finbook.reason','Rollback-only integration verification',true)", admin)
	employee := id("INSERT INTO finbook.employees(employee_code,name,official_email,date_of_joining,reporting_to) VALUES('EMP','Test employee','employee@example.test','2026-01-01',$1) RETURNING id", adminEmployee)
	user := id("INSERT INTO finbook.users(employee_id,username,display_name,login_email,password_hash,password_reset_required) VALUES($1,'employee','Test employee','employee@example.test','rollback-only-test-hash',false) RETURNING id", employee)
	reject("23505", "INSERT INTO finbook.employees(employee_code,name,official_email) VALUES('EMP','Other','other@example.test')")
	reject("23505", "INSERT INTO finbook.employees(employee_code,name,official_email) VALUES('OTHER','Other','EMPLOYEE@example.test')")
	reject("23514", "INSERT INTO finbook.employees(employee_code,name) VALUES('NO-EMAIL','Missing email')")
	reject("23514", "UPDATE finbook.employees SET reporting_to=id WHERE id=$1", employee)
	reject("23514", "UPDATE finbook.employees SET date_of_leaving='2025-12-31' WHERE id=$1", employee)
	reject("23505", "INSERT INTO finbook.users(employee_id,username,display_name,login_email,password_hash) VALUES($1,'second','Second','second@example.test','test-hash')", employee)
	reject("23514", "UPDATE finbook.users SET employee_id=$2 WHERE id=$1", user, adminEmployee)

	exec("INSERT INTO finbook.sessions(user_id,token_hash,expires_at) VALUES($1,decode(repeat('ab',32),'hex'),now()+interval '1 hour')", user)
	exec("UPDATE finbook.users SET account_status='Locked' WHERE id=$1", user)
	expect("SELECT account_status='Locked' AND NOT active FROM finbook.users WHERE id=$1", user)
	expect("SELECT employment_status='Active' FROM finbook.employees WHERE id=$1", employee)
	expect("SELECT revoked_at IS NOT NULL FROM finbook.sessions WHERE user_id=$1", user)
	exec("UPDATE finbook.employees SET employment_status='Inactive' WHERE id=$1", employee)
	expect("SELECT account_status='Inactive' AND NOT active FROM finbook.users WHERE id=$1", user)
	reject("23514", "UPDATE finbook.users SET account_status='Active' WHERE id=$1", user)
	reject("23514", "UPDATE finbook.employees SET reporting_to=$2 WHERE id=$1", adminEmployee, employee)
	exec("UPDATE finbook.employees SET employment_status='Active' WHERE id=$1", employee)
	expect("SELECT account_status='Inactive' FROM finbook.users WHERE id=$1", user)
	exec("UPDATE finbook.users SET account_status='Active' WHERE id=$1", user)

	firstRole := id("INSERT INTO finbook.roles(code,name) VALUES('TEST-ONE','Test role one') RETURNING id")
	secondRole := id("INSERT INTO finbook.roles(code,name) VALUES('TEST-TWO','Test role two') RETURNING id")
	exec("INSERT INTO finbook.user_roles(user_id,role_id,created_by) VALUES($1,$2,$3),($1,$4,$3)", user, firstRole, admin, secondRole)
	expect("SELECT count(*)=2 FROM finbook.user_roles WHERE user_id=$1", user)
	exec("DELETE FROM finbook.user_roles WHERE user_id=$1 AND role_id=$2", user, secondRole)
	exec("UPDATE finbook.user_roles SET role_id=$2 WHERE user_id=$1", user, secondRole)
	expect("SELECT count(*)=4 AND count(*) FILTER(WHERE action='Assigned')=2 AND count(*) FILTER(WHERE action='Removed')=1 AND count(*) FILTER(WHERE action='Modified')=1 FROM finbook.role_history WHERE user_id=$1", user)
	expect("SELECT bool_and(modified_by=$2::uuid AND employee_id=$3::uuid) FROM finbook.role_history WHERE user_id=$1", user, admin, employee)
	reject("55000", "UPDATE finbook.role_history SET remarks='tampered'")
	reject("55000", "DELETE FROM finbook.role_history")
	reject("55000", "TRUNCATE finbook.role_history")

	request := id("INSERT INTO finbook.employee_change_requests(employee_id,requested_by,employee_version,original_values,requested_values) SELECT id,$2,version,'{\"personal_email\":null}','{\"personal_email\":\"new@example.test\"}' FROM finbook.employees WHERE id=$1 RETURNING id", employee, user)
	expect("SELECT personal_email IS NULL FROM finbook.employees WHERE id=$1", employee)
	reject("23514", "INSERT INTO finbook.employee_change_requests(employee_id,requested_by,employee_version,original_values,requested_values) VALUES($1,$2,1,'{}','{\"employment_status\":\"Active\"}')", employee, user)
	reject("23514", "INSERT INTO finbook.employee_change_requests(employee_id,requested_by,employee_version,original_values,requested_values) VALUES($1,$2,1,'{}','{\"personal_email\":\"other@example.test\"}')", adminEmployee, user)
	reject("23514", "UPDATE finbook.employee_change_requests SET status='Approved',decided_by=$2,decision_remarks='Self approval' WHERE id=$1", request, user)
	exec("UPDATE finbook.employee_change_requests SET status='Rejected',decided_by=$2,decision_remarks='Test rejection' WHERE id=$1", request, admin)
	expect("SELECT personal_email IS NULL FROM finbook.employees WHERE id=$1", employee)
	reject("23514", "UPDATE finbook.employee_change_requests SET status='Approved',decided_by=$2,decision_remarks='Changed decision' WHERE id=$1", request, admin)
	reject("55000", "DELETE FROM finbook.employee_change_requests WHERE id=$1", request)
	exec("INSERT INTO finbook.password_reset_tokens(user_id,token_hash,expires_at,created_by) VALUES($1,decode(repeat('cd',32),'hex'),now()+interval '10 minutes',$2)", user, admin)
	expect("SELECT NOT EXISTS(SELECT 1 FROM finbook.audit_log WHERE new_values ?| ARRAY['password_hash','token_hash','photo_bytes'] OR old_values ?| ARRAY['password_hash','token_hash','photo_bytes'])")
	expect("SELECT EXISTS(SELECT 1 FROM finbook.audit_log WHERE actor_id=$1 AND entity_table='employees')", admin)
	reject("55000", "UPDATE finbook.audit_log SET reason='tampered'")
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal("rollback failed")
	}
	var exists bool
	if err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", schema).Scan(&exists); err != nil || exists {
		t.Fatal("test schema must never persist")
	}
}
