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

// Explicit opt-in. Every fixture and its isolated schema are rolled back;
// existing finbook/public data is never used as test data.
func TestPostgreSQLIntegrity(t *testing.T) {
	if os.Getenv("FINBOOK_DB_INTEGRATION") != "1" {
		t.Skip("requires explicit cloud integration opt-in")
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("database test configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	conn, err := Connect(ctx, config)
	if err != nil {
		t.Fatal("cloud test connection failed; raw credentials suppressed")
	}
	defer func() {
		closeCtx, release := context.WithTimeout(context.Background(), 3*time.Second)
		defer release()
		_ = conn.Close(closeCtx)
	}()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal("cannot start rollback-only test transaction")
	}
	defer func() {
		rollbackCtx, release := context.WithTimeout(context.Background(), 5*time.Second)
		defer release()
		_ = tx.Rollback(rollbackCtx)
	}()
	schema := fmt.Sprintf("finbook_test_%d", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, strings.ReplaceAll(sql, "finbook", schema), args...); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				t.Fatalf("database integrity fixture failed (SQLSTATE %s)", pgErr.Code)
			}
			t.Fatal("database integrity fixture failed")
		}
	}
	exec(bootstrapSQL)
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range all {
		if _, err := tx.Exec(ctx, strings.ReplaceAll(migration.SQL, "finbook", schema)); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				t.Fatalf("migration %s validation failed (SQLSTATE %s; SQL position %d)", migration.Name, pgErr.Code, pgErr.Position)
			}
			t.Fatalf("migration %s validation failed", migration.Name)
		}
	}
	queryID := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.QueryRow(ctx, strings.ReplaceAll(sql, "finbook", schema), args...).Scan(&id); err != nil {
			t.Fatal("fixture identity lookup failed")
		}
		return id
	}
	reject := func(code, sql string, args ...any) {
		t.Helper()
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal("cannot create test savepoint")
		}
		_, failure := savepoint.Exec(ctx, strings.ReplaceAll(sql, "finbook", schema), args...)
		_ = savepoint.Rollback(ctx)
		var pgErr *pgconn.PgError
		if !errors.As(failure, &pgErr) || pgErr.Code != code {
			t.Fatalf("expected integrity rejection SQLSTATE %s", code)
		}
	}
	exec("INSERT INTO finbook.currencies(code,name,decimal_places) VALUES('INR','Synthetic test currency',2)")
	company := queryID("INSERT INTO finbook.company(legal_name,base_currency) VALUES('SYNTHETIC TEST ONLY','INR') RETURNING id")
	branch := queryID("INSERT INTO finbook.branches(company_id,code,name) VALUES($1,'TEST','Synthetic') RETURNING id", company)
	employee := queryID("INSERT INTO finbook.employees(employee_code,name,official_email) VALUES('TEST-ACTOR','Synthetic actor','actor@example.test') RETURNING id")
	actor := queryID("INSERT INTO finbook.users(username,display_name,password_hash,employee_id,login_email) VALUES('synthetic','Synthetic','test-only-placeholder',$1,'actor@example.test') RETURNING id", employee)
	year := queryID("INSERT INTO finbook.financial_years(label,starts_on,ends_on) VALUES('TEST','2026-04-01','2027-03-31') RETURNING id")
	period := queryID("INSERT INTO finbook.accounting_periods(financial_year_id,label,starts_on,ends_on) VALUES($1,'TEST-OCT','2026-10-01','2026-10-31') RETURNING id", year)
	closed := queryID("INSERT INTO finbook.accounting_periods(financial_year_id,label,starts_on,ends_on,status) VALUES($1,'TEST-NOV','2026-11-01','2026-11-30','closed') RETURNING id", year)
	debit := queryID("INSERT INTO finbook.accounts(code,name,classification) VALUES('TEST-A','Synthetic asset','asset') RETURNING id")
	credit := queryID("INSERT INTO finbook.accounts(code,name,classification) VALUES('TEST-I','Synthetic income','income') RETURNING id")
	kind := queryID("INSERT INTO finbook.voucher_types(code,name) VALUES('TEST','Synthetic journal') RETURNING id")
	voucher := queryID("INSERT INTO finbook.vouchers(branch_id,voucher_type_id,voucher_number,posting_date,period_id,currency_code,idempotency_key,payload_sha256) VALUES($1,$2,'TEST-1','2026-10-15',$3,'INR','synthetic-event',decode(repeat('ab',32),'hex')) RETURNING id", branch, kind, period)
	exec("INSERT INTO finbook.voucher_lines(voucher_id,line_number,account_id,debit,credit) VALUES($1,1,$2,100,0),($1,2,$3,0,99)", voucher, debit, credit)
	reject("23514", "UPDATE finbook.vouchers SET status='posted',posted_by=$2,posted_at=now() WHERE id=$1", voucher, actor)
	exec("UPDATE finbook.voucher_lines SET credit=100 WHERE voucher_id=$1 AND line_number=2", voucher)
	reject("23514", "UPDATE finbook.vouchers SET posting_date='2026-11-15',period_id=$2,status='posted',posted_by=$3,posted_at=now() WHERE id=$1", voucher, closed, actor)
	reject("23505", "INSERT INTO finbook.vouchers(branch_id,voucher_type_id,voucher_number,posting_date,period_id,currency_code,idempotency_key,payload_sha256) VALUES($1,$2,'TEST-DUP','2026-10-15',$3,'INR','synthetic-event',decode(repeat('ab',32),'hex'))", branch, kind, period)
	exec("UPDATE finbook.vouchers SET status='posted',posted_by=$2,posted_at=now() WHERE id=$1", voucher, actor)
	reject("55000", "UPDATE finbook.voucher_lines SET debit=99 WHERE voucher_id=$1 AND line_number=1", voucher)
	reject("55000", "UPDATE finbook.vouchers SET narration='tampered' WHERE id=$1", voucher)
	reject("55000", "DELETE FROM finbook.vouchers WHERE id=$1", voucher)
	reject("55000", "UPDATE finbook.accounts SET classification='expense' WHERE id=$1", credit)
	reversing := queryID("INSERT INTO finbook.vouchers(branch_id,voucher_type_id,voucher_number,posting_date,period_id,currency_code,idempotency_key,payload_sha256,reversal_of) VALUES($1,$2,'TEST-REV','2026-10-16',$3,'INR','synthetic-reversal',decode(repeat('cd',32),'hex'),$4) RETURNING id", branch, kind, period, voucher)
	exec("INSERT INTO finbook.voucher_lines(voucher_id,line_number,account_id,debit,credit) VALUES($1,1,$2,10,0),($1,2,$3,0,10)", reversing, credit, debit)
	reject("23514", "UPDATE finbook.vouchers SET status='posted',posted_by=$2,posted_at=now() WHERE id=$1", reversing, actor)
	exec("UPDATE finbook.voucher_lines SET debit=CASE WHEN line_number=1 THEN 100 ELSE 0 END,credit=CASE WHEN line_number=2 THEN 100 ELSE 0 END WHERE voucher_id=$1", reversing)
	exec("UPDATE finbook.vouchers SET status='posted',posted_by=$2,posted_at=now() WHERE id=$1", reversing, actor)
	exec("UPDATE finbook.vouchers SET status='reversed',reversed_by=$2,reversal_reason='Synthetic test reversal' WHERE id=$1", voucher, reversing)
	var nonzeroBalances int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+schema+".trial_balance WHERE signed_balance<>0").Scan(&nonzeroBalances); err != nil || nonzeroBalances != 0 {
		t.Fatal("linked reversal must exactly clear the original account balances")
	}
	reject("23514", "SELECT 1.0000001::finbook.money_amount")
	exec("INSERT INTO finbook.tickets(ticket_number) VALUES('123-4567890123')")
	reject("23505", "INSERT INTO finbook.tickets(ticket_number) VALUES('1234567890123')")
	reject("23514", "INSERT INTO finbook.tax_rules(code,version,component,rate_percent,basis,effective_from,active) VALUES('UNVERIFIED',1,'TEST',NULL,'{}','2026-10-01',true)")
	reject("55000", "UPDATE finbook.audit_log SET reason='tampered'")
	reject("55000", "TRUNCATE finbook.audit_log")
	var balanced bool
	if err = tx.QueryRow(ctx, "SELECT sum(debit)=sum(credit) FROM "+schema+".posted_entries").Scan(&balanced); err != nil || !balanced {
		t.Fatal("posted ledger does not balance")
	}
	var secretLeaked bool
	if err = tx.QueryRow(ctx, "SELECT exists(SELECT 1 FROM "+schema+".audit_log WHERE new_values ? 'password_hash')").Scan(&secretLeaked); err != nil || secretLeaked {
		t.Fatal("audit payload must redact password hashes")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal("test rollback failed")
	}
	var persisted bool
	if err = conn.QueryRow(ctx, "SELECT exists(SELECT 1 FROM pg_namespace WHERE nspname=$1)", schema).Scan(&persisted); err != nil || persisted {
		t.Fatal("integration fixtures must not persist")
	}
}
