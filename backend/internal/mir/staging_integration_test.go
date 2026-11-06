package mir

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"voltafinbook/backend/internal/database"
	"voltafinbook/backend/internal/mir/decoder"
)

// Explicit opt-in: creates and drops ONLY a fresh, uniquely named test database.
// The configured application database is never migrated, seeded or deleted.
func TestMySQLStageIntegration(t *testing.T) {
	if os.Getenv("FINBOOK_MIR_INTEGRATION") != "1" {
		t.Skip("set FINBOOK_MIR_INTEGRATION=1 for an isolated MySQL database")
	}
	if os.Getenv("FINBOOK_ROOT") == "" {
		root, err := filepath.Abs("../../..")
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("FINBOOK_ROOT", root)
	}
	config, e := database.LoadConfig()
	if e != nil {
		t.Fatal("database configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	admin, e := database.Connect(ctx, config)
	if e != nil {
		t.Fatal("MySQL connection unavailable")
	}
	defer admin.Close()
	var suffix [8]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		t.Fatal(e)
	}
	schema := "finbook_mir_test_" + hex.EncodeToString(suffix[:])
	if !regexp.MustCompile(`^finbook_mir_test_[0-9a-f]{16}$`).MatchString(schema) {
		t.Fatal("unsafe test schema")
	}
	if _, e = admin.ExecContext(ctx, "CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); e != nil {
		var dbErr *mysql.MySQLError
		if errors.As(e, &dbErr) {
			t.Fatalf("cannot create isolated MySQL test database (MySQL error %d)", dbErr.Number)
		}
		t.Fatal("cannot create isolated MySQL test database")
	}
	defer func() {
		cleanupCtx, release := context.WithTimeout(context.Background(), 15*time.Second)
		defer release()
		if _, e := admin.ExecContext(cleanupCtx, "DROP DATABASE `"+schema+"`"); e != nil {
			t.Errorf("could not remove isolated test database %s", schema)
		}
	}()
	isolated := config.Clone()
	isolated.DBName = schema
	db, e := database.Connect(ctx, isolated)
	if e != nil {
		t.Fatal("isolated database unavailable")
	}
	defer db.Close()
	status, e := database.ApplyMigrations(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	if !status.Current || status.Applied != 6 {
		t.Fatalf("migration status %+v", status)
	}
	t.Log("Applied migrations 0001, 0004, 0005, 0006, 0007, 0008 in isolated database")
	// Import timestamps must remain UTC even when the MySQL session is local time.
	timestampConn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = timestampConn.ExecContext(ctx, `SET time_zone='+05:30'`); e != nil {
		timestampConn.Close()
		t.Fatal(e)
	}
	stamp, e := timestampConn.ExecContext(ctx, `INSERT INTO mir_import_batch(errors) VALUES(JSON_ARRAY())`)
	if e != nil {
		timestampConn.Close()
		t.Fatal(e)
	}
	stampID, e := stamp.LastInsertId()
	if e != nil {
		timestampConn.Close()
		t.Fatal(e)
	}
	var seconds int
	if e = timestampConn.QueryRowContext(ctx, `SELECT ABS(TIMESTAMPDIFF(SECOND,created_at,UTC_TIMESTAMP(6))) FROM mir_import_batch WHERE id=?`, stampID).Scan(&seconds); e != nil || seconds > 5 {
		timestampConn.Close()
		t.Fatal("import timestamp did not use UTC")
	}
	if _, e = timestampConn.ExecContext(ctx, `SET time_zone='+00:00'`); e != nil {
		timestampConn.Close()
		t.Fatal(e)
	}
	timestampConn.Close()
	employee := bytes.Repeat([]byte{1}, 16)
	user := bytes.Repeat([]byte{2}, 16)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := db.ExecContext(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO employees(id,employee_code,name) VALUES(?,'SYNTHETIC-MIR','Synthetic MIR test user')`, employee)
	exec(`INSERT INTO users(id,employee_id,username,display_name,password_hash,password_reset_required) VALUES(?,?,'synthetic-mir','Synthetic MIR test user','not-a-login-password',FALSE)`, user, employee)
	tokenBytes := bytes.Repeat([]byte{3}, 32)
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	exec(`INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 HOUR))`, bytes.Repeat([]byte{4}, 16), user, hash[:])
	folder := t.TempDir()
	settings := filepath.Join(t.TempDir(), "mir-source.json")
	if e := os.WriteFile(settings, []byte(jsonText(Config{Path: folder, Locked: true})), 0600); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(NewStageHandler(isolated, &Service{ConfigFile: settings}))
	defer server.Close()
	// Use the existing cookie name from the application contract.
	requestAuth := func(method, path, contentType string, body io.Reader) (int, []byte) {
		t.Helper()
		req, e := http.NewRequest(method, server.URL+path, body)
		if e != nil {
			t.Fatal(e)
		}
		req.AddCookie(&http.Cookie{Name: "finbook_session", Value: token})
		req.Header.Set("Content-Type", contentType)
		res, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, raw
	}

	upload := func(name string, data []byte) Batch {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("files", name)
		if e != nil {
			t.Fatal(e)
		}
		part.Write(data)
		writer.Close()
		code, raw := requestAuth("POST", "/api/v1/mir/import", writer.FormDataContentType(), &body)
		if code != 201 {
			t.Fatalf("import status %d: %s", code, raw)
		}
		var b Batch
		if e := json.Unmarshal(raw, &b); e != nil {
			t.Fatal(e)
		}
		return b
	}
	methodCode, methodBody := requestAuth("PUT", "/api/v1/mir/records", "", nil)
	if methodCode != 405 || !strings.Contains(string(methodBody), `"error":"method_not_allowed"`) {
		t.Fatal("inconsistent method error")
	}
	raw := syntheticStageRecord()
	batch := upload("SYNTHETIC-STAGING.MIR", raw)
	if batch.Imported != 1 || batch.Failed != 0 {
		t.Fatalf("batch %+v", batch)
	}
	duplicate := upload("RENAMED-SYNTHETIC.MIR", raw)
	if duplicate.Duplicates != 1 || duplicate.Imported != 0 {
		t.Fatalf("duplicate batch %+v", duplicate)
	}
	code, body := requestAuth("GET", "/api/v1/mir/records?from_date=2026-11-05&to_date=2026-11-05", "", nil)
	if code != 200 {
		t.Fatalf("list %d: %s", code, body)
	}
	var listed struct {
		Records []StageRow
		Total   int
	}
	if e = json.Unmarshal(body, &listed); e != nil || listed.Total != 1 {
		t.Fatalf("MIR date list: %s", body)
	}
	id := listed.Records[0].ID
	if listed.Records[0].MIRDate != "2026-11-05" || listed.Records[0].FirstTravelDate != "2026-12-10" {
		t.Fatal("MIR and travel dates conflated")
	}
	_, body = requestAuth("GET", "/api/v1/mir/records?from_date=2026-12-10&to_date=2026-12-10", "", nil)
	json.Unmarshal(body, &listed)
	if listed.Total != 0 {
		t.Fatal("date filter used travel date")
	}
	repo := Repository{db}
	detail, e := repo.Detail(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	if len(detail.Typed["passenger"]) != 2 || len(detail.Typed["fare"]) != 1 || len(detail.Typed["fare_tax"]) != 1 {
		t.Fatalf("typed child counts: passenger=%d fare=%d tax=%d", len(detail.Typed["passenger"]), len(detail.Typed["fare"]), len(detail.Typed["fare_tax"]))
	}
	if len(detail.Relationships) != 6 {
		t.Fatalf("expected 2 passenger fares, basis fare/segment, 2 passenger segments; got %d", len(detail.Relationships))
	}
	if detail.RawHex != hex.EncodeToString(raw) {
		t.Fatal("raw source altered")
	}
	code, body = requestAuth("GET", fmt.Sprintf("/api/v1/mir/records/%d", id), "", nil)
	if code != 200 {
		t.Fatalf("detail API %d %s", code, body)
	}
	// Capture expands one MIR into passenger/ticket rows and filters by ticket
	// issue date, independently of MIR creation and extraction dates.
	requestAuthCapture := func(query string, want int) []CaptureRow {
		t.Helper()
		status, raw := requestAuth("GET", "/api/v1/mir/capture?"+query, "", nil)
		if status != 200 {
			t.Fatalf("capture %d: %s", status, raw)
		}
		var result struct {
			Records []CaptureRow
			Total   int
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Total != want || len(result.Records) != want {
			t.Fatalf("capture expected %d rows: %s", want, raw)
		}
		return result.Records
	}
	captured := requestAuthCapture("from_date=2026-11-04&to_date=2026-11-04&date_basis=issue", 2)
	if captured[0].IssueDate != "2026-11-04" || captured[0].PNR != "SYN001" || captured[0].GDS != "1G" || captured[0].InvoiceNumber != "INV000001" || captured[0].Fare != "120.00" || captured[0].TicketNumber == captured[1].TicketNumber || strings.Contains(captured[0].PassengerName, "(+") {
		t.Fatalf("capture did not preserve ticket-level values: %+v", captured)
	}
	// Proceed resolves the selected passenger against the persisted source,
	// never the booking summary or a neighboring passenger's fare.
	draftURL := fmt.Sprintf("/api/v1/mir/capture/%d/draft?record_id=%d", captured[1].PassengerID, id)
	draftStatus, draftBody := requestAuth("GET", draftURL, "", nil)
	var draft CaptureDraft
	if draftStatus != 200 || json.Unmarshal(draftBody, &draft) != nil {
		t.Fatalf("selected ticket draft %d: %s", draftStatus, draftBody)
	}
	if draft.Fields["ticketNumber"].Value != captured[1].TicketNumber || draft.Fields["passengerName"].Value != captured[1].PassengerName || draft.Fields["mirTotalFare"].Value != "120.00" || len(draft.RawLines) == 0 {
		t.Fatal("Proceed mixed passenger values or lost source evidence")
	}
	if _, exists := draft.Fields["clientTotal"]; exists {
		t.Fatal("source fare treated as client selling price")
	}
	wrongStatus, _ := requestAuth("GET", fmt.Sprintf("/api/v1/mir/capture/%d/draft?record_id=999999", captured[1].PassengerID), "", nil)
	if wrongStatus != 404 {
		t.Fatal("Proceed accepted mismatched record and passenger IDs")
	}
	if captured[0].WebReference != "" {
		t.Fatal("unmapped web reference was invented")
	}
	detailsURL := fmt.Sprintf("/api/v1/mir/capture/%d/details?record_id=%d", captured[1].PassengerID, id)
	detailStatus, detailBody := requestAuth("GET", detailsURL, "", nil)
	if detailStatus != 200 || !bytes.Contains(detailBody, []byte(`"customer_mapping"`)) || !bytes.Contains(detailBody, []byte(`"taxes"`)) {
		t.Fatalf("ticket review details missing: %d %s", detailStatus, detailBody)
	}
	// Persist a real linked original sale. Existing unique keys must block a
	// second sale, including differently punctuated versions of the same ticket.
	companyID, branchID, ticketID, saleID, lineID := bytes.Repeat([]byte{10}, 16), bytes.Repeat([]byte{11}, 16), bytes.Repeat([]byte{12}, 16), bytes.Repeat([]byte{13}, 16), bytes.Repeat([]byte{14}, 16)
	exec(`INSERT INTO currencies(code,name,decimal_places) VALUES('USD','US dollar',2)`)
	exec(`INSERT INTO airlines(id,code,name) VALUES(?,'ZZ','Synthetic airline')`, bytes.Repeat([]byte{19}, 16))
	readyRows := []CaptureRow{{TicketNumber: "9991234567890", PassengerName: "TEST/READY", PNR: "READY1", IssueDate: "2026-11-04", Sector: "AAA-BBB", Fare: "100.00", Currency: "USD", airlineSource: "ZZ", importStatus: "staged", validationJSON: []byte("[]")}}
	if err := repo.enrichCapture(ctx, readyRows); err != nil || readyRows[0].Progress.Status != "Ready for Sale" {
		t.Fatalf("validated active master mappings not ready: %#v %v", readyRows, err)
	}
	exec(`UPDATE currencies SET active=FALSE WHERE code='USD'`)
	if err := repo.enrichCapture(ctx, readyRows); err != nil || readyRows[0].Progress.Status != "Needs Review" {
		t.Fatal("inactive currency incorrectly ready for sale")
	}
	exec(`UPDATE currencies SET active=TRUE WHERE code='USD'`)
	exec(`INSERT INTO company(id,legal_name,base_currency) VALUES(?,'Synthetic test company','USD')`, companyID)
	exec(`INSERT INTO branches(id,company_id,code,name) VALUES(?,?,'TEST','Test branch')`, branchID, companyID)
	exec(`INSERT INTO tickets(id,ticket_number) VALUES(?,?)`, ticketID, captured[1].TicketNumber)
	exec(`INSERT INTO sales_documents(id,company_id,branch_id,sales_class,document_date,currency_code) VALUES(?,?,?,'international','2026-11-04','USD')`, saleID, companyID, branchID)
	exec(`INSERT INTO sales_lines(id,document_id,line_number,ticket_id,service_category) VALUES(?,?,1,?,'air')`, lineID, saleID, ticketID)
	if _, err := db.ExecContext(ctx, `INSERT INTO tickets(id,ticket_number) VALUES(?,?)`, bytes.Repeat([]byte{15}, 16), captured[1].TicketNumber[:3]+"-"+captured[1].TicketNumber[3:]); err == nil {
		t.Fatal("duplicate normalized ticket number accepted")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sales_lines(id,document_id,line_number,ticket_id,service_category) VALUES(?,?,2,?,'air')`, bytes.Repeat([]byte{16}, 16), saleID, ticketID); err == nil {
		t.Fatal("second original sale for same ticket accepted")
	}
	duplicateStatus, _ := requestAuth("GET", draftURL, "", nil)
	if duplicateStatus != 409 {
		t.Fatal("backend prepared another draft for an existing sale")
	}
	linkedRows := requestAuthCapture("from_date=2026-11-04&to_date=2026-11-04&date_basis=issue", 2)
	for _, row := range linkedRows {
		if row.PassengerID == captured[1].PassengerID && (row.Progress.Status != "Sale Created" || row.Progress.LinkedSale == nil || row.Progress.CanPrepareDraft || row.Progress.CanViewSale) {
			t.Fatalf("linked sale status/permissions incorrect: %#v", row.Progress)
		}
	}
	saleURL := fmt.Sprintf("/api/v1/mir/capture/%d/sale?record_id=%d", captured[1].PassengerID, id)
	denied, _ := requestAuth("GET", saleURL, "", nil)
	if denied != 403 {
		t.Fatal("sales read without grant allowed")
	}
	roleID, permissionID := bytes.Repeat([]byte{17}, 16), bytes.Repeat([]byte{18}, 16)
	exec(`INSERT INTO roles(id,code,name) VALUES(?,'SALES-TEST','Sales test')`, roleID)
	exec(`INSERT INTO permissions(id,module,screen,action) VALUES(?,'sales','workspace','view')`, permissionID)
	exec(`INSERT INTO user_roles(user_id,role_id) VALUES(?,?)`, user, roleID)
	exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES(?,?)`, roleID, permissionID)
	allowed, saleBody := requestAuth("GET", saleURL, "", nil)
	if allowed != 200 || !bytes.Contains(saleBody, []byte(`"currency":"USD"`)) {
		t.Fatalf("permitted sale view failed: %d %s", allowed, saleBody)
	}
	exec(`UPDATE roles SET active=FALSE WHERE id=?`, roleID)
	denied, _ = requestAuth("GET", saleURL, "", nil)
	if denied != 403 {
		t.Fatal("inactive role retained sales view access")
	}
	// A cancelled or deleted original sale still reserves the original ticket.
	exec(`UPDATE sales_documents SET status='cancelled',deleted_at=UTC_TIMESTAMP(6) WHERE id=?`, saleID)
	duplicateStatus, _ = requestAuth("GET", draftURL, "", nil)
	if duplicateStatus != 409 {
		t.Fatal("cancelled/soft-deleted original sale allowed duplicate")
	}
	requestAuthCapture("from_date=2026-11-05&to_date=2026-11-05&date_basis=issue", 0)
	// Imported_at is UTC and independent of the MIR date.
	if _, err := db.ExecContext(ctx, "UPDATE mir_record SET imported_at='2026-11-07 23:59:59' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	requestAuthCapture("from_date=2026-11-07&to_date=2026-11-07&date_basis=processing", 2)
	requestAuthCapture("from_date=2026-11-08&to_date=2026-11-08&date_basis=processing", 0)
	t.Setenv("BUSINESS_TIMEZONE", "Asia/Kolkata")
	requestAuthCapture("from_date=2026-11-07&to_date=2026-11-07&date_basis=processing", 0)
	requestAuthCapture("from_date=2026-11-08&to_date=2026-11-08&date_basis=processing", 2)
	// Date-only issue dates do not shift with the business timezone.
	requestAuthCapture("from_date=2026-11-04&to_date=2026-11-04&date_basis=issue", 2)
	t.Setenv("BUSINESS_TIMEZONE", "UTC")
	if _, err := db.ExecContext(ctx, "UPDATE mir_passenger SET decoded_data=JSON_SET(decoded_data,'$.A02DTE','31FEB26') WHERE record_id=?", id); err != nil {
		t.Fatal(err)
	}
	requestAuthCapture("from_date=2026-01-01&to_date=2026-12-31&date_basis=issue", 0)
	invalidDates := requestAuthCapture("from_date=2026-11-07&to_date=2026-11-07&date_basis=processing", 2)
	if invalidDates[0].IssueDate != "" || invalidDates[0].IssueDateSource != "31FEB26" {
		t.Fatal("invalid source issue date was inferred")
	}
	for i := 0; i < 2; i++ {
		code, body = requestAuth("POST", fmt.Sprintf("/api/v1/mir/records/%d/reprocess", id), "", nil)
		if code != 200 {
			t.Fatalf("reprocess %d: %s", code, body)
		}
	}
	staleStatus, _ := requestAuth("GET", draftURL, "", nil)
	if staleStatus != 404 {
		t.Fatal("Proceed accepted passenger identity replaced by reprocessing")
	}
	var count int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_source_file`).Scan(&count)
	if count != 1 {
		t.Fatal("reprocessing duplicated source")
	}
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_passenger WHERE record_id=?`, id).Scan(&count)
	if count != 2 {
		t.Fatal("reprocessing duplicated children")
	}
	// Force a uniqueness error after parent and children are written. The entire
	// first import and replacement transaction must roll back.
	parsed, e := decoder.ParseStream(raw, "SYNTHETIC-ROLLBACK.MIR")
	if e != nil {
		t.Fatal(e)
	}
	parsed[0].Items = append(parsed[0].Items, parsed[0].Items[0])
	if _, e = repo.Stage(ctx, detail.SourceFileID, 2, parsed[0], 0); e == nil {
		t.Fatal("expected duplicate-section transaction failure")
	}
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_record`).Scan(&count)
	if count != 1 {
		t.Fatal("failed insert left parent")
	}
	if _, e = repo.Stage(ctx, detail.SourceFileID, 1, parsed[0], id); e == nil {
		t.Fatal("expected replacement failure")
	}
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_passenger WHERE record_id=?`, id).Scan(&count)
	if count != 2 {
		t.Fatal("failed replacement did not restore previous children")
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO mir_section_item(section_id,record_id,item_number,raw_data) VALUES(999999,999999,1,'x')`); e == nil {
		t.Fatal("orphan foreign key accepted")
	}
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_section_item i LEFT JOIN mir_record r ON r.id=i.record_id LEFT JOIN mir_section s ON s.id=i.section_id WHERE r.id IS NULL OR s.id IS NULL`).Scan(&count)
	if count != 0 {
		t.Fatal("orphan children found")
	}
	for _, table := range []string{"sales", "vouchers", "ledger_entries"} {
		var exists int
		if e := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, table).Scan(&exists); e != nil {
			t.Fatal(e)
		}
		if exists > 0 {
			if e := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); e != nil || count != 0 {
				t.Fatal("staging wrote accounting data")
			}
		}
	}
	_, body = requestAuth("GET", fmt.Sprintf("/api/v1/mir/batches/%d", batch.ID), "", nil)
	var history Batch
	json.Unmarshal(body, &history)
	if history.Imported != 1 {
		t.Fatal("batch summary mismatch")
	}
	stream := append(bytes.Clone(raw), raw...)
	streamBatch := upload("SYNTHETIC-STREAM.MIR", stream)
	if streamBatch.Imported != 2 || streamBatch.Failed != 0 {
		t.Fatalf("stream counts %+v", streamBatch)
	}
	streamDuplicate := upload("SYNTHETIC-STREAM-COPY.MIR", stream)
	if streamDuplicate.Duplicates != 2 {
		t.Fatal("stream duplicate count")
	}
	failedBatch := upload("SYNTHETIC-MALFORMED.MIR", []byte("T5truncated"))
	if failedBatch.Failed != 1 || len(failedBatch.Errors) != 1 {
		t.Fatal("record validation errors not returned")
	}
	// Simulate an interrupted worker after source storage but before its first
	// record transaction. Retry must resume that source instead of duplicating it.
	retryRaw := bytes.Clone(raw)
	copy(retryRaw[98:], "RETRY1")
	retryBatch, e := repo.StartBatch(ctx, user)
	if e != nil {
		t.Fatal(e)
	}
	retrySource, dup, e := repo.Source(ctx, retryBatch, "SYNTHETIC-RETRY.MIR", retryRaw)
	if e != nil || dup {
		t.Fatal("retry source setup")
	}
	retried := upload("SYNTHETIC-RETRY.MIR", retryRaw)
	if retried.Imported != 1 || retried.Duplicates != 0 {
		t.Fatal("interrupted source could not resume")
	}
	if e := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_record WHERE source_file_id=?`, retrySource).Scan(&count); e != nil || count != 1 {
		t.Fatal("retry source relationship")
	}
	// Explicit folder pulls reuse import/staging and preserve source files.
	if status, _ := requestAuth("POST", "/api/v1/mir/pull", "", nil); status != 400 {
		t.Fatal("empty folder should not create a batch")
	}
	pullRaw := bytes.Clone(raw)
	copy(pullRaw[98:], "PULL01")
	sourcePath := filepath.Join(folder, "PULL.MIR")
	if err := os.WriteFile(sourcePath, pullRaw, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		status, body := requestAuth("POST", "/api/v1/mir/pull", "", nil)
		var pulled Batch
		if status != 201 || json.Unmarshal(body, &pulled) != nil {
			t.Fatalf("folder pull failed %d: %s", status, body)
		}
		if (i == 0 && pulled.Imported != 1) || (i == 1 && pulled.Duplicates != 1) {
			t.Fatalf("folder pull counts %+v", pulled)
		}
	}
	if data, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(data, pullRaw) {
		t.Fatal("folder pull changed source bytes")
	}
	// One private regression file, never copied to the repository or logs.
	sample := os.Getenv("MIR_REGRESSION_FILE")
	if sample == "" {
		sample = filepath.Join("..", "..", "..", "..", "IDC-MIR-Reader", "reference", "sample_files", "AABYXGAL.MIR")
	}
	if data, e := os.ReadFile(sample); e == nil {
		b := upload("AABYXGAL.MIR", data)
		if b.Imported != 1 || b.Failed != 0 {
			t.Fatalf("private regression import: %d staged, %d failed", b.Imported, b.Failed)
		}
		t.Log("AABYXGAL.MIR actual authenticated HTTP import passed")
	}
	t.Log("Authenticated HTTP import, duplicate protection, date filters, detail, batches, reprocessing, rollback and foreign keys passed")
}

func syntheticStageRecord() []byte {
	h1 := []byte(strings.Repeat(" ", 80))
	copy(h1, "T51G7733920000000001")
	copy(h1[20:], "05NOV26")
	copy(h1[34:], "123")
	copy(h1[61:], "10DEC26")
	h2 := []byte(strings.Repeat(" ", 63))
	copy(h2[17:], "SYN001")
	pax := func(name, ticket string) string {
		b := []byte(strings.Repeat(" ", 79))
		copy(b, "A02"+name)
		copy(b[48:], ticket)
		copy(b[75:], "07")
		copy(b[60:], "INV000001")
		return string(b) + "\rTD:04NOV26\r"
	}
	segment := []byte(strings.Repeat(" ", 103))
	copy(segment, "A0401ZZ999SYNTHETIC")
	fare := []byte(strings.Repeat(" ", 50))
	copy(fare, "A0707USD")
	copy(fare[8:], "      100.00")
	copy(fare[20:], "USD      120.00")
	basis := []byte(strings.Repeat(" ", 43))
	copy(basis, "A080701SYNTHETIC")
	body := pax("SYNTHETIC/ONE", "1234567890") + pax("SYNTHETIC/TWO", "1234567891") + "\r" + string(segment) + "\r\r" + string(fare) + "T1:   20.00US\r\r" + string(basis) + "\r\r"
	raw := bytes.Join([][]byte{h1, h2, []byte(strings.Repeat(" ", 96)), []byte(strings.Repeat(" ", 51)), []byte(strings.Repeat("0", 48)), {}, []byte(body)}, []byte{13})
	copy(raw[10:15], fmt.Sprintf("%05d", len(raw)))
	return raw
}
