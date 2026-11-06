package reconciliation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"voltafinbook/backend/internal/database"
)

func TestMySQLStatementLifecycle(t *testing.T) {
	if os.Getenv("FINBOOK_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("requires isolated MySQL integration opt-in")
	}
	config, err := database.LoadConfig()
	if err != nil {
		t.Fatal("test database configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := database.Connect(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	schema := "finbook_reconciliation_test_" + hex.EncodeToString(suffix[:])
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, release := context.WithTimeout(context.Background(), 15*time.Second)
		defer release()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE `"+schema+"`"); err != nil {
			t.Error(err)
		}
	}()
	isolated := config.Clone()
	isolated.DBName = schema
	db, err := database.Connect(ctx, isolated)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Verify additive setup for an installation without migration history.
	if ready, err := database.SetupReconciliation(ctx, db, false); err != nil || ready {
		t.Fatalf("unexpected plan: %v %v", ready, err)
	}
	for i := 0; i < 2; i++ {
		if ready, err := database.SetupReconciliation(ctx, db, true); err != nil || !ready {
			t.Fatalf("setup is not repeatable: %v %v", ready, err)
		}
	}
	// Exercise the full migration path in a clean schema as well.
	for _, table := range []string{"reconciliation_statement_rows", "reconciliation_statements"} {
		if _, err = db.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	employee := bytes.Repeat([]byte{1}, 16)
	user := bytes.Repeat([]byte{2}, 16)
	exec(`INSERT INTO employees(id,employee_code,name) VALUES(?,'STATEMENT-TEST','Statement test employee')`, employee)
	exec(`INSERT INTO users(id,employee_id,username,display_name,password_hash,password_reset_required) VALUES(?,?,'statement-test','Statement test user','not-a-login-hash',FALSE)`, user, employee)
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	hash := sha256.Sum256([]byte(token))
	exec(`INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 HOUR))`, bytes.Repeat([]byte{4}, 16), user, hash[:])
	server := httptest.NewServer(NewHandler(isolated))
	defer server.Close()
	request := func(method, path, contentType string, body io.Reader, authenticated bool, origin string, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+"/api/v1/reconciliation/statements"+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if authenticated {
			req.AddCookie(&http.Cookie{Name: "finbook_session", Value: token})
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.StatusCode, want, raw)
		}
		return raw
	}
	request("GET", "", "", nil, false, "", 401)
	upload := func(name string, data []byte, want int) struct {
		Statement Statement
		Duplicate bool
	} {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
		form.WriteField("kind", "bank")
		form.Close()
		raw := request("POST", "", form.FormDataContentType(), &body, true, "", want)
		var result struct {
			Statement Statement
			Duplicate bool
		}
		if err = json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	source := []byte("Date,Reference,Debit,Credit\n06/11/2026,0000123,1234.50,\n")
	first := upload("bank.csv", source, 201)
	if first.Duplicate || first.Statement.RowCount != 2 {
		t.Fatalf("bad upload: %#v", first)
	}
	duplicate := upload("renamed.csv", source, 200)
	if !duplicate.Duplicate || duplicate.Statement.ID != first.Statement.ID {
		t.Fatal("duplicate source created another import")
	}
	detailRaw := request("GET", "/"+first.Statement.ID, "", nil, true, "", 200)
	var detail struct {
		Statement Statement
		Rows      []Row
	}
	json.Unmarshal(detailRaw, &detail)
	if len(detail.Rows) != 2 || detail.Rows[1].Cells[1] != "0000123" || detail.Rows[1].Cells[2] != "1234.50" || detail.Statement.UploadedBy != "02020202-0202-0202-0202-020202020202" {
		t.Fatalf("stored source values or actor lost: %s", detailRaw)
	}
	upload("statement.pdf", testPDF("BT /F1 12 Tf 40 750 Td (Statement REF001 1234.50) Tj ET"), 201)
	book := excelize.NewFile()
	book.SetSheetRow("Sheet1", "A1", &[]string{"Reference", "Amount"})
	book.SetSheetRow("Sheet1", "A2", &[]string{"0001", "20.00"})
	workbook, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	book.Close()
	upload("statement.xlsx", workbook.Bytes(), 201)
	upload("scan.pdf", testPDF(""), 422)
	request("GET", "/"+first.Statement.ID+"?offset=-1", "", nil, true, "", 400)
	request("GET", "/999999999", "", nil, true, "", 404)
	request("POST", "", "", nil, true, "https://elsewhere.invalid", 403)
	var count int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reconciliation_statements").Scan(&count)
	if count != 3 {
		t.Fatalf("invalid or duplicate upload persisted: count=%d", count)
	}
	// Force the child insert to fail and verify that the parent and file roll back.
	exec(`CREATE TRIGGER reconciliation_test_fail BEFORE INSERT ON reconciliation_statement_rows FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic failure'`)
	_, _, err = (Repository{db}).Save(ctx, "failure.csv", "bank", hex.EncodeToString(user), []byte("different-source"), []Row{{Number: 1, Section: "CSV", SourceRow: 1, Cells: []string{"test"}}})
	if err == nil {
		t.Fatal("expected insertion failure")
	}
	exec(`DROP TRIGGER reconciliation_test_fail`)
	var after int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reconciliation_statements").Scan(&after)
	if after != count {
		t.Fatal("failed import left a partial statement")
	}
	var stored []byte
	if err = db.QueryRowContext(ctx, "SELECT source_bytes FROM reconciliation_statements WHERE id=?", first.Statement.ID).Scan(&stored); err != nil || !bytes.Equal(stored, source) {
		t.Fatal("original statement bytes were not preserved")
	}
	pageRows := make([]Row, 101)
	for i := range pageRows {
		pageRows[i] = Row{Number: i + 1, Section: "CSV", SourceRow: i + 1, Cells: []string{"value"}}
	}
	paged, _, err := (Repository{db}).Save(ctx, "pages.csv", "supplier", "test-actor", []byte("pagination-source"), pageRows)
	if err != nil {
		t.Fatal(err)
	}
	_, lastPage, err := (Repository{db}).Detail(ctx, paged.ID, 100)
	if err != nil || len(lastPage) != 1 || lastPage[0].Number != 101 {
		t.Fatalf("saved row pagination failed: %#v %v", lastPage, err)
	}
}
