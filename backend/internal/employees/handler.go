package employees

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"

	mysql "github.com/go-sql-driver/mysql"
)

const maxRequestBody = 16 << 10

type Handler struct {
	config *mysql.Config
}

type Employee struct {
	ID               string `json:"id"`
	EmployeeCode     string `json:"employeeCode"`
	Name             string `json:"name"`
	OfficialEmail    string `json:"officialEmail"`
	MobileNumber     string `json:"mobileNumber"`
	Designation      string `json:"designation"`
	Department       string `json:"department"`
	EmploymentStatus string `json:"employmentStatus"`
	DateOfJoining    string `json:"dateOfJoining"`
	DateOfLeaving    string `json:"dateOfLeaving"`
	Version          int    `json:"version"`
}

type employeeInput struct {
	EmployeeCode     string `json:"employeeCode"`
	Name             string `json:"name"`
	OfficialEmail    string `json:"officialEmail"`
	MobileNumber     string `json:"mobileNumber"`
	Designation      string `json:"designation"`
	Department       string `json:"department"`
	EmploymentStatus string `json:"employmentStatus"`
	DateOfJoining    string `json:"dateOfJoining"`
	DateOfLeaving    string `json:"dateOfLeaving"`
	Version          int    `json:"version,omitempty"`
}

func NewHandler(config *mysql.Config, _ bool) http.Handler {
	handler := &Handler{config: config}
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/employees/me", auth.RequireAuthenticated(config, http.HandlerFunc(handler.me)))
	mux.Handle("GET /api/v1/employees", auth.RequireAuthenticated(config, http.HandlerFunc(handler.list)))
	mux.Handle("POST /api/v1/employees", auth.RequireAuthenticated(config, http.HandlerFunc(handler.create)))
	mux.Handle("PATCH /api/v1/employees/{id}", auth.RequireAuthenticated(config, http.HandlerFunc(handler.update)))
	handler.registerAccess(mux)
	return mux
}

func (handler *Handler) open(ctx context.Context) (*sql.DB, error) {
	if handler == nil || handler.config == nil {
		return nil, errors.New("database not configured")
	}
	return database.Connect(ctx, handler.config)
}

func (handler *Handler) list(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id,employee_code,name,official_email,mobile_number,
		designation,department,employment_status,date_of_joining,date_of_leaving,version
		FROM employees WHERE deleted_at IS NULL ORDER BY name,employee_code LIMIT 1000`)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer rows.Close()
	result := make([]Employee, 0)
	for rows.Next() {
		var id []byte
		var email, mobile, designation, department sql.NullString
		var joining, leaving sql.NullTime
		var employee Employee
		if err := rows.Scan(&id, &employee.EmployeeCode, &employee.Name, &email, &mobile,
			&designation, &department, &employee.EmploymentStatus, &joining, &leaving, &employee.Version); err != nil {
			writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
			return
		}
		employee.ID, err = formatUUID(id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
			return
		}
		employee.OfficialEmail = email.String
		employee.MobileNumber = mobile.String
		employee.Designation = designation.String
		employee.Department = department.String
		employee.DateOfJoining = dateValue(joining)
		employee.DateOfLeaving = dateValue(leaving)
		result = append(result, employee)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"employees": result, "capabilities": map[string]bool{"create": true, "update": true},
	})
}

func (handler *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	employeeID, err := parseUUID(principal.EmployeeID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer db.Close()
	employee, err := getEmployee(ctx, db, employeeID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "employee_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]Employee{"employee": employee})
}

func getEmployee(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id []byte) (Employee, error) {
	var employee Employee
	var email, mobile, designation, department sql.NullString
	var joining, leaving sql.NullTime
	var storedID []byte
	err := queryer.QueryRowContext(ctx, `SELECT id,employee_code,name,official_email,mobile_number,
		designation,department,employment_status,date_of_joining,date_of_leaving,version
		FROM employees WHERE id=? AND deleted_at IS NULL`, id).Scan(&storedID, &employee.EmployeeCode,
		&employee.Name, &email, &mobile, &designation, &department, &employee.EmploymentStatus,
		&joining, &leaving, &employee.Version)
	if err != nil {
		return Employee{}, err
	}
	employee.ID, err = formatUUID(storedID)
	if err != nil {
		return Employee{}, err
	}
	employee.OfficialEmail = email.String
	employee.MobileNumber = mobile.String
	employee.Designation = designation.String
	employee.Department = department.String
	employee.DateOfJoining = dateValue(joining)
	employee.DateOfLeaving = dateValue(leaving)
	return employee, nil
}

func (handler *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input employeeInput
	if !decodeInput(w, r, &input) {
		return
	}
	input, err := validate(input, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_employee")
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	actorID, err := principalBytes(principal, ok)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	id, err := newUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "employee_create_failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	employeeID, _ := parseUUID(id)
	joining := nullableDate(input.DateOfJoining)
	leaving := nullableDate(input.DateOfLeaving)
	active := input.EmploymentStatus == "Active"
	_, err = tx.ExecContext(ctx, `INSERT INTO employees
		(id,employee_code,name,official_email,mobile_number,designation,department,employment_status,
		date_of_joining,date_of_leaving,active,version,created_by,updated_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,1,?,?)`, employeeID, input.EmployeeCode, input.Name,
		nullableString(input.OfficialEmail), nullableString(input.MobileNumber), nullableString(input.Designation),
		nullableString(input.Department), input.EmploymentStatus, joining, leaving, active, actorID, actorID)
	if err != nil {
		writeError(w, http.StatusConflict, "employee_create_conflict")
		return
	}
	created, err := getEmployee(ctx, tx, employeeID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	if err := writeAudit(ctx, tx, actorID, "create", created.ID, nil, created); err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_audit_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]Employee{"employee": created})
}

func (handler *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	employeeID, err := parseUUID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_employee_id")
		return
	}
	var input employeeInput
	if !decodeInput(w, r, &input) {
		return
	}
	input, err = validate(input, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_employee")
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	actorID, err := principalBytes(principal, ok)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	old, err := getEmployee(ctx, tx, employeeID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "employee_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_service_unavailable")
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE employees SET employee_code=?,name=?,official_email=?,mobile_number=?,
		designation=?,department=?,employment_status=?,date_of_joining=?,date_of_leaving=?,active=?,
		version=version+1,updated_by=?,updated_at=UTC_TIMESTAMP(6)
		WHERE id=? AND version=? AND deleted_at IS NULL`, input.EmployeeCode, input.Name,
		nullableString(input.OfficialEmail), nullableString(input.MobileNumber), nullableString(input.Designation),
		nullableString(input.Department), input.EmploymentStatus, nullableDate(input.DateOfJoining),
		nullableDate(input.DateOfLeaving), input.EmploymentStatus == "Active", actorID, employeeID, input.Version)
	if err != nil {
		writeError(w, http.StatusConflict, "employee_update_conflict")
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeError(w, http.StatusConflict, "employee_update_conflict")
		return
	}
	updated, err := getEmployee(ctx, tx, employeeID)
	if err != nil {
		writeError(w, http.StatusConflict, "employee_update_conflict")
		return
	}
	if updated.Version == old.Version {
		writeError(w, http.StatusConflict, "employee_update_conflict")
		return
	}
	if err := writeAudit(ctx, tx, actorID, "update", updated.ID, old, updated); err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_audit_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "employee_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]Employee{"employee": updated})
}

func writeAudit(ctx context.Context, tx *sql.Tx, actorID []byte, event, employeeID string, oldValue, newValue any) error {
	var oldJSON, newJSON any
	if oldValue != nil {
		value, err := json.Marshal(oldValue)
		if err != nil {
			return err
		}
		oldJSON = string(value)
	}
	value, err := json.Marshal(newValue)
	if err != nil {
		return err
	}
	newJSON = string(value)
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log
		(actor_id,database_actor,event,entity_table,entity_key,old_values,new_values)
		VALUES(?,CURRENT_USER(),?,'employees',?,CAST(? AS JSON),CAST(? AS JSON))`,
		actorID, event, employeeID, oldJSON, newJSON)
	return err
}

func validate(input employeeInput, update bool) (employeeInput, error) {
	input.EmployeeCode = strings.TrimSpace(input.EmployeeCode)
	input.Name = strings.TrimSpace(input.Name)
	input.OfficialEmail = strings.TrimSpace(input.OfficialEmail)
	input.MobileNumber = strings.TrimSpace(input.MobileNumber)
	input.Designation = strings.TrimSpace(input.Designation)
	input.Department = strings.TrimSpace(input.Department)
	input.EmploymentStatus = strings.TrimSpace(input.EmploymentStatus)
	if input.EmployeeCode == "" || len(input.EmployeeCode) > 100 || input.Name == "" || len(input.Name) > 191 {
		return input, errors.New("invalid employee identity")
	}
	if len(input.OfficialEmail) > 254 || (input.OfficialEmail != "" && !validEmail(input.OfficialEmail)) ||
		len(input.MobileNumber) > 64 || len(input.Designation) > 100 || len(input.Department) > 100 {
		return input, errors.New("invalid employee contact or organization details")
	}
	if input.EmploymentStatus != "Active" && input.EmploymentStatus != "Inactive" && input.EmploymentStatus != "Resigned" {
		return input, errors.New("invalid employment status")
	}
	joining, err := parseDate(input.DateOfJoining)
	if err != nil {
		return input, err
	}
	leaving, err := parseDate(input.DateOfLeaving)
	if err != nil {
		return input, err
	}
	if joining != nil && leaving != nil && leaving.Before(*joining) {
		return input, errors.New("leaving date is before joining date")
	}
	if update && input.Version < 1 {
		return input, errors.New("version is required")
	}
	if !update && input.Version != 0 {
		return input, errors.New("version is managed by the server")
	}
	return input, nil
}

func decodeInput(w http.ResponseWriter, r *http.Request, input *employeeInput) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func validEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func parseDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, errors.New("invalid date")
	}
	return &date, nil
}

func nullableDate(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func dateValue(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func principalBytes(principal auth.Principal, ok bool) ([]byte, error) {
	if !ok {
		return nil, errors.New("principal missing")
	}
	return parseUUID(principal.ID)
}

func parseUUID(value string) ([]byte, error) {
	value = strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 16 {
		return nil, errors.New("invalid UUID")
	}
	return decoded, nil
}

func formatUUID(value []byte) (string, error) {
	if len(value) != 16 {
		return "", errors.New("invalid binary UUID")
	}
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return formatUUID(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
