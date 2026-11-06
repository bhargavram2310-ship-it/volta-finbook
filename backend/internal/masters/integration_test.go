package masters

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"voltafinbook/backend/internal/database"
)

// Creates and drops only a fresh test database; never writes application data.
func TestMySQLMasterLifecycle(t *testing.T) {
	if os.Getenv("FINBOOK_MASTERS_INTEGRATION") != "1" {
		t.Skip("requires isolated MySQL integration opt-in")
	}
	root, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("FINBOOK_ROOT", root)
	config, e := database.LoadConfig()
	if e != nil {
		t.Fatal("test database configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	admin, e := database.Connect(ctx, config)
	if e != nil {
		t.Fatal("test MySQL unavailable")
	}
	defer admin.Close()
	var suffix [8]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		t.Fatal(e)
	}
	schema := "finbook_masters_test_" + hex.EncodeToString(suffix[:])
	if !regexp.MustCompile(`^finbook_masters_test_[a-f0-9]{16}$`).MatchString(schema) {
		t.Fatal("invalid test schema")
	}
	if _, e = admin.ExecContext(ctx, "CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, release := context.WithTimeout(context.Background(), 15*time.Second)
		defer release()
		if _, e := admin.ExecContext(cleanup, "DROP DATABASE `"+schema+"`"); e != nil {
			t.Error("test schema cleanup failed")
		}
	}()
	isolated := config.Clone()
	isolated.DBName = schema
	db, e := database.Connect(ctx, isolated)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = database.ApplyMigrations(ctx, db); e != nil {
		t.Fatal(e)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := db.ExecContext(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	employee := bytes.Repeat([]byte{1}, 16)
	user := bytes.Repeat([]byte{2}, 16)
	exec(`INSERT INTO employees(id,employee_code,name) VALUES(?,'SYNTHETIC','Test employee')`, employee)
	exec(`INSERT INTO users(id,employee_id,username,display_name,password_hash,password_reset_required) VALUES(?,?,'synthetic','Test user','not-a-login-hash',FALSE)`, user, employee)
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	hash := sha256.Sum256([]byte(token))
	exec(`INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 HOUR))`, bytes.Repeat([]byte{4}, 16), user, hash[:])
	mux := http.NewServeMux()
	mux.Handle("/api/v1/masters/sales-lookups", SalesLookups(isolated))
	mux.Handle("/api/v1/masters/", NewHandler(isolated))
	server := httptest.NewServer(mux)
	defer server.Close()
	request := func(method, path string, input any, want int) []byte {
		t.Helper()
		var body io.Reader
		if input != nil {
			raw, e := json.Marshal(input)
			if e != nil {
				t.Fatal(e)
			}
			body = bytes.NewReader(raw)
		}
		r, e := http.NewRequest(method, server.URL+"/api/v1/masters/"+path, body)
		if e != nil {
			t.Fatal(e)
		}
		r.AddCookie(&http.Cookie{Name: "finbook_session", Value: token})
		r.Header.Set("Content-Type", "application/json")
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.StatusCode, want, raw)
		}
		return raw
	}
	create := func(kind string, values map[string]string) Record {
		t.Helper()
		raw := request("POST", kind, mutation{Values: values}, 201)
		var result struct{ Record Record }
		if e := json.Unmarshal(raw, &result); e != nil {
			t.Fatal(e)
		}
		return result.Record
	}
	list := func(kind string) []Record {
		t.Helper()
		raw := request("GET", kind, nil, 200)
		var result struct{ Records []Record }
		if e := json.Unmarshal(raw, &result); e != nil {
			t.Fatal(e)
		}
		return result.Records
	}
	update := func(kind string, record Record) Record {
		t.Helper()
		raw := request("PATCH", kind+"/"+record.ID, mutation{Values: record.Values, Revision: record.Revision}, 200)
		var result struct{ Record Record }
		if e := json.Unmarshal(raw, &result); e != nil {
			t.Fatal(e)
		}
		return result.Record
	}
	deleteRecord := func(kind string, record Record, status int) {
		t.Helper()
		request("DELETE", kind+"/"+record.ID, mutation{Revision: record.Revision}, status)
	}
	for kind := range catalog {
		if kind == "sales-types" {
			if len(list(kind)) != 3 {
				t.Fatal("default sales types missing")
			}
			continue
		}
		if records := list(kind); len(records) != 0 {
			t.Fatal("test masters should start empty")
		}
	}
	salesType := list("sales-types")[0]
	salesType.Values["name"] = "International travel"
	salesType = update("sales-types", salesType)
	salesType.Values["sales_class"] = "domestic"
	request("PATCH", "sales-types/"+salesType.ID, mutation{Values: salesType.Values, Revision: salesType.Revision}, 400)
	salesType.Values["sales_class"] = "international"
	deleteRecord("sales-types", salesType, 200)
	create("sales-types", salesType.Values)
	inr := create("currencies", map[string]string{"code": "INR", "name": "Test rupee", "decimal_places": "2", "active": "true"})
	usd := create("currencies", map[string]string{"code": "USD", "name": "Test dollar", "decimal_places": "2", "active": "true"})
	company := create("company", map[string]string{"legal_name": "Test company", "base_currency": "INR"})
	branch := create("branches", map[string]string{"code": "BR1", "name": "Test branch", "company_id": company.ID, "active": "true"})
	customer := create("customers", map[string]string{"code": "C1", "legal_name": "Test customer", "company_id": company.ID, "active": "true", "credit_days": "30", "credit_limit": "123.123456"})
	agent := create("agents", map[string]string{"code": "A1", "legal_name": "Test agent", "company_id": company.ID, "active": "true"})
	supplier := create("suppliers", map[string]string{"code": "S1", "legal_name": "Test supplier", "company_id": company.ID, "active": "true", "settlement_channel": "bsp"})
	airline := create("airlines", map[string]string{"code": "XX", "name": "Test airline", "active": "true", "ticket_prefix": "999"})
	location := create("locations", map[string]string{"code": "IN", "name": "Test country", "kind": "country", "active": "true"})
	city := create("locations", map[string]string{"code": "CITY", "name": "Test city", "kind": "city", "parent_id": location.ID, "active": "true"})
	account := create("accounts", map[string]string{"code": "BANK", "name": "Test bank ledger", "classification": "asset", "postable": "true", "active": "true"})
	bank := create("bank-cash", map[string]string{"ledger_account_id": account.ID, "kind": "bank", "currency_code": "INR", "bank_name": "Test bank", "active": "true"})
	year := create("financial-years", map[string]string{"label": "2026-27", "starts_on": "2026-04-01", "ends_on": "2027-03-31", "closed": "false"})
	period := create("periods", map[string]string{"financial_year_id": year.ID, "label": "November", "starts_on": "2026-11-01", "ends_on": "2026-11-30", "status": "open"})

	rate := create("exchange-rates", map[string]string{"from_currency": "USD", "to_currency": "INR", "effective_date": "2026-11-05", "rate": "83.123456789012"})
	tax := create("tax", map[string]string{"code": "TAX", "version": "1", "component": "Test component", "rate_percent": "5.123456", "basis": "{}", "effective_from": "2026-11-05"})
	rule := create("commercial-rules", map[string]string{"code": "REFUND", "kind": "refund", "configuration": "{}"})
	// All types can be read, edited, and deleted with exact decimal round trips.
	fixtures := map[string]Record{"branches": branch, "customers": customer, "agents": agent, "suppliers": supplier, "airlines": airline, "bank-cash": bank, "periods": period, "exchange-rates": rate, "tax": tax, "commercial-rules": rule}
	for kind, record := range fixtures {
		if len(list(kind)) != 1 {
			t.Fatalf("missing %s", kind)
		}
		fixtures[kind] = update(kind, record)
	}
	if fixtures["customers"].Values["credit_limit"] != "123.123456" || fixtures["exchange-rates"].Values["rate"] != "83.123456789012" {
		t.Fatal("decimal precision changed")
	}
	if fixtures["periods"].Values["starts_on"] != "2026-11-01" {
		t.Fatal("date round trip failed")
	}
	// A stale editor cannot overwrite a newer edit or delete it.
	airline.Values["name"] = "Renamed airline"
	current := update("airlines", airline)
	request("PATCH", "airlines/"+airline.ID, mutation{Values: airline.Values, Revision: airline.Revision}, 409)
	deleteRecord("airlines", airline, 409)
	request("POST", "airlines", mutation{Values: airline.Values}, 409)
	fixtures["airlines"] = current
	// Relationships prevent destructive deletes; precision is protected when used.
	deleteRecord("company", company, 409)
	deleteRecord("currencies", inr, 409)
	deleteRecord("accounts", account, 409)
	deleteRecord("financial-years", year, 409)
	deleteRecord("locations", location, 409)
	inr.Values["decimal_places"] = "3"
	request("PATCH", "currencies/INR", mutation{Values: inr.Values, Revision: inr.Revision}, 409)
	location.Values["parent_id"] = city.ID
	request("PATCH", "locations/"+location.ID, mutation{Values: location.Values, Revision: location.Revision}, 400)
	// Master changes flow into sales lookup lists, with inactive entries excluded.
	var lookups map[string][]Option
	json.Unmarshal(request("GET", "sales-lookups", nil, 200), &lookups)
	if len(lookups["airlines"]) != 1 || len(lookups["customers"]) != 1 || len(lookups["currencies"]) != 2 || len(lookups["suppliers"]) != 1 {
		t.Fatal("sales lookups missing masters")
	}
	current.Values["active"] = "false"
	fixtures["airlines"] = update("airlines", current)
	json.Unmarshal(request("GET", "sales-lookups", nil, 200), &lookups)
	if len(lookups["airlines"]) != 0 {
		t.Fatal("inactive airline in sales lookup")
	}
	// Orphan party cleanup allows the deleted unused customer code to be reused.
	deleteRecord("customers", fixtures["customers"], 200)
	fixtures["customers"] = create("customers", customer.Values)
	for kind, record := range fixtures {
		deleteRecord(kind, record, 200)
		if len(list(kind)) != 0 {
			t.Fatalf("deleted %s still listed", kind)
		}
	}
	deleteRecord("locations", city, 200)
	location.Values["parent_id"] = ""
	deleteRecord("locations", location, 200)
	deleteRecord("accounts", account, 200)
	deleteRecord("financial-years", year, 200)
	deleteRecord("company", company, 200)
	deleteRecord("currencies", inr, 200)
	deleteRecord("currencies", usd, 200)
	var created, updated, deleted int
	if e := db.QueryRowContext(ctx, `SELECT SUM(event='create'),SUM(event='update'),SUM(event='delete') FROM audit_log WHERE entity_table IN ('airlines','customers')`).Scan(&created, &updated, &deleted); e != nil {
		t.Fatal(e)
	}
	if created < 3 || updated < 2 || deleted < 3 {
		t.Fatal("audit events missing")
	}
	t.Log("All master types passed create/edit/delete, FK protection, exact decimals, dates, lookup filtering, stale edits, and audit checks")
}
