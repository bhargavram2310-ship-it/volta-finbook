package employees

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"voltafinbook/backend/internal/database"
)

func TestEmployeeValidationNormalizesAndChecksFields(t *testing.T) {
	input := employeeInput{
		EmployeeCode: " EMP-01 ", Name: " Employee One ", OfficialEmail: "one@example.invalid ",
		EmploymentStatus: "Active", DateOfJoining: "2025-01-01",
	}
	normalized, err := validate(input, false)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.EmployeeCode != "EMP-01" || normalized.Name != "Employee One" || normalized.OfficialEmail != "one@example.invalid" {
		t.Fatalf("employee values were not normalized: %#v", normalized)
	}
}

func TestEmployeeValidationRejectsInvalidInputs(t *testing.T) {
	tests := []employeeInput{
		{EmployeeCode: "EMP-01", Name: "Employee", EmploymentStatus: "Unknown"},
		{EmployeeCode: "EMP-01", Name: "Employee", OfficialEmail: "not-an-email", EmploymentStatus: "Active"},
		{EmployeeCode: "EMP-01", Name: "Employee", EmploymentStatus: "Active", DateOfJoining: "2025-02-01", DateOfLeaving: "2025-01-01"},
		{EmployeeCode: "EMP-01", Name: "Employee", EmploymentStatus: "Active", Version: 1},
	}
	for _, input := range tests {
		if _, err := validate(input, false); err == nil {
			t.Errorf("invalid create input was accepted: %#v", input)
		}
	}
	if _, err := validate(employeeInput{EmployeeCode: "EMP-01", Name: "Employee", EmploymentStatus: "Active"}, true); err == nil {
		t.Fatal("an update without the expected record version was accepted")
	}
}

func TestEmployeeRoutesRequireAuthenticatedSession(t *testing.T) {
	handler := NewHandler(nil, false)
	for _, testCase := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/employees/me", ""},
		{http.MethodGet, "/api/v1/employees", ""},
		{http.MethodPost, "/api/v1/employees", `{"employeeCode":"EMP","name":"Employee","employmentStatus":"Active"}`},
	} {
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got status %d, want unauthorized", testCase.method, testCase.path, response.Code)
		}
	}
}

func TestUUIDParsingRejectsInvalidIdentifiers(t *testing.T) {
	for _, value := range []string{"", "not-a-uuid", "001122"} {
		if _, err := parseUUID(value); err == nil {
			t.Errorf("invalid UUID %q was accepted", value)
		}
	}
}

func TestMySQLEmployeeAPIIntegration(t *testing.T) {
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
	actorEmployeeID, actorUserID, sessionID := make([]byte, 16), make([]byte, 16), make([]byte, 16)
	for _, id := range [][]byte{actorEmployeeID, actorUserID, sessionID} {
		if _, err := rand.Read(id); err != nil {
			t.Fatal(err)
		}
	}
	suffix, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	suffix = strings.ReplaceAll(suffix, "-", "")
	employeeCode := "API-EMP-" + suffix
	username := "api-actor-" + suffix
	if _, err := tx.ExecContext(ctx, `INSERT INTO employees(id,employee_code,name,employment_status,active,version)
		VALUES(?,?,?,'Active',TRUE,1)`, actorEmployeeID, "API-ACTOR-"+suffix, "API Test Actor"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users
		(id,employee_id,username,display_name,password_hash,password_reset_required,account_status,active,version)
		VALUES(?,?,?,?,?,FALSE,'Active',TRUE,1)`, actorUserID, actorEmployeeID,
		username, "API Test Actor", "integration-placeholder-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET user_id=? WHERE id=?`, actorUserID, actorEmployeeID); err != nil {
		t.Fatal(err)
	}
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	tokenHash := sha256.Sum256([]byte(token))
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at)
		VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 HOUR))`, sessionID, actorUserID, tokenHash[:]); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	createdEmployeeID := ""
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		var rawCreatedEmployeeID []byte
		err := db.QueryRowContext(cleanupCtx, `SELECT id FROM employees WHERE employee_code=?`, employeeCode).Scan(&rawCreatedEmployeeID)
		if err == nil {
			id, formatErr := formatUUID(rawCreatedEmployeeID)
			if formatErr != nil {
				t.Errorf("format employee fixture ID for cleanup: %v", formatErr)
			} else {
				createdEmployeeID = id
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("find created employee fixture for cleanup: %v", err)
		}
		if createdEmployeeID != "" {
			if _, err := db.ExecContext(cleanupCtx, `DELETE FROM audit_log WHERE entity_table='employees' AND entity_key=?`, createdEmployeeID); err != nil {
				t.Errorf("clean employee audit fixture: %v", err)
			}
			if _, err := db.ExecContext(cleanupCtx, `DELETE FROM employees WHERE id=?`, rawCreatedEmployeeID); err != nil {
				t.Errorf("clean created employee fixture: %v", err)
			}
		}
		for _, cleanup := range []struct {
			label string
			query string
			args  []any
		}{
			{"sessions", `DELETE FROM sessions WHERE id=?`, []any{sessionID}},
		} {
			if _, err := db.ExecContext(cleanupCtx, cleanup.query, cleanup.args...); err != nil {
				t.Errorf("clean %s fixture: %v", cleanup.label, err)
			}
		}
		if _, err := db.ExecContext(cleanupCtx, `UPDATE employees SET user_id=NULL WHERE id=?`, actorEmployeeID); err != nil {
			t.Errorf("unlink test actor employee: %v", err)
		}
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=?`, actorUserID); err != nil {
			t.Errorf("clean test user fixture: %v", err)
		}
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM employees WHERE id=?`, actorEmployeeID); err != nil {
			t.Errorf("clean test actor employee fixture: %v", err)
		}
	}()

	handler := NewHandler(config, false)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: "finbook_session", Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	profileResponse := call(http.MethodGet, "/api/v1/employees/me", "")
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("self profile status %d: %s", profileResponse.Code, profileResponse.Body.String())
	}
	directoryResponse := call(http.MethodGet, "/api/v1/employees", "")
	if directoryResponse.Code != http.StatusOK || !strings.Contains(directoryResponse.Body.String(), `"create":true`) {
		t.Fatalf("authenticated employee directory was unavailable: %d %s", directoryResponse.Code, directoryResponse.Body.String())
	}
	createBody := `{"employeeCode":"` + employeeCode + `","name":"API Test Employee","officialEmail":"api-employee-` + suffix + `@example.invalid","employmentStatus":"Active"}`
	createResponse := call(http.MethodPost, "/api/v1/employees", createBody)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("employee create status %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var createdResponse struct {
		Employee Employee `json:"employee"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &createdResponse); err != nil {
		t.Fatal(err)
	}
	createdEmployeeID = createdResponse.Employee.ID
	if createdEmployeeID == "" || createdResponse.Employee.Version != 1 {
		t.Fatalf("unexpected created employee response: %#v", createdResponse.Employee)
	}
	updateBody := `{"employeeCode":"` + employeeCode + `","name":"API Test Employee Updated","officialEmail":"api-employee-` + suffix + `@example.invalid","employmentStatus":"Active","version":1}`
	updateResponse := call(http.MethodPatch, "/api/v1/employees/"+createdEmployeeID, updateBody)
	if updateResponse.Code != http.StatusOK || !strings.Contains(updateResponse.Body.String(), `"version":2`) {
		t.Fatalf("employee update status %d: %s", updateResponse.Code, updateResponse.Body.String())
	}
	conflictResponse := call(http.MethodPatch, "/api/v1/employees/"+createdEmployeeID, updateBody)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("stale employee version was not rejected: %d %s", conflictResponse.Code, conflictResponse.Body.String())
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE actor_id=?
		AND entity_table='employees' AND entity_key=?`, actorUserID, createdEmployeeID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("expected two employee audit entries, got %d", auditCount)
	}
}
