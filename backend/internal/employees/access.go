package employees

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"voltafinbook/backend/internal/auth"
)

type User struct {
	ID            string   `json:"id"`
	EmployeeID    string   `json:"employeeId"`
	EmployeeName  string   `json:"employeeName"`
	EmployeeCode  string   `json:"employeeCode"`
	Username      string   `json:"username"`
	LoginMethod   string   `json:"loginMethod"`
	DisplayName   string   `json:"displayName"`
	LoginEmail    string   `json:"loginEmail"`
	AccountStatus string   `json:"accountStatus"`
	RoleIDs       []string `json:"roleIds"`
	Version       int      `json:"version"`
}

type userInput struct {
	EmployeeID    string   `json:"employeeId"`
	Username      string   `json:"username"`
	LoginMethod   string   `json:"loginMethod"`
	DisplayName   string   `json:"displayName"`
	LoginEmail    string   `json:"loginEmail"`
	AccountStatus string   `json:"accountStatus"`
	Password      string   `json:"password,omitempty"`
	RoleIDs       []string `json:"roleIds"`
	Remarks       string   `json:"remarks"`
	Version       int      `json:"version,omitempty"`
}

func (h *Handler) registerAccess(mux *http.ServeMux) {
	register := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, auth.RequireAuthenticated(h.config, fn))
	}
	register("GET /api/v1/users", h.listUsers)
	register("GET /api/v1/users/options", h.userOptions)
	register("POST /api/v1/users", h.saveUser)
	register("PATCH /api/v1/users/{id}", h.saveUser)
	register("GET /api/v1/roles", h.listRoles)
	register("POST /api/v1/roles", h.saveRole)
	register("PATCH /api/v1/roles/{id}", h.saveRole)
}

func (h *Handler) accessDB(w http.ResponseWriter, r *http.Request) (*sql.DB, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	db, err := h.open(ctx)
	if err != nil {
		cancel()
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return nil, nil, nil
	}
	return db, ctx, cancel
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	db, ctx, cancel := h.accessDB(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT u.id,u.employee_id,e.name,e.employee_code,u.username,u.login_method,u.display_name,COALESCE(u.login_email,''),u.account_status,u.version,COALESCE(ur.role_ids,'')
		FROM users u JOIN employees e ON e.id=u.employee_id
		LEFT JOIN (SELECT user_id,GROUP_CONCAT(LOWER(HEX(role_id)) ORDER BY role_id SEPARATOR ',') AS role_ids
			FROM user_roles GROUP BY user_id) ur ON ur.user_id=u.id
		WHERE u.deleted_at IS NULL ORDER BY u.display_name`)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return
	}
	users := []User{}
	for rows.Next() {
		var user User
		var id, employee []byte
		var roleIDs string
		if err = rows.Scan(&id, &employee, &user.EmployeeName, &user.EmployeeCode, &user.Username, &user.LoginMethod, &user.DisplayName, &user.LoginEmail, &user.AccountStatus, &user.Version, &roleIDs); err != nil {
			rows.Close()
			writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
			return
		}
		user.ID, err = formatUUID(id)
		if err == nil {
			user.EmployeeID, err = formatUUID(employee)
		}
		if err != nil {
			rows.Close()
			writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
			return
		}
		user.RoleIDs, err = parseRoleIDList(roleIDs)
		if err != nil {
			rows.Close()
			writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
			return
		}
		users = append(users, user)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return
	}
	actor, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": users, "currentUserId": actor.ID, "capabilities": map[string]bool{"create": true, "update": true},
	})
}

func (h *Handler) userOptions(w http.ResponseWriter, r *http.Request) {
	db, ctx, cancel := h.accessDB(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT e.id,e.employee_code,e.name,e.employment_status,COALESCE(e.official_email,''),COALESCE(LOWER(HEX(u.id)),'') FROM employees e
		LEFT JOIN users u ON u.employee_id=e.id AND u.deleted_at IS NULL WHERE e.deleted_at IS NULL ORDER BY e.name`)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return
	}
	options := []map[string]string{}
	for rows.Next() {
		var id []byte
		var code, name, status, email, user string
		if err = rows.Scan(&id, &code, &name, &status, &email, &user); err != nil {
			break
		}
		employeeID, formatErr := formatUUID(id)
		if formatErr != nil {
			err = formatErr
			break
		}
		if user != "" {
			raw, parseErr := parseUUID(user)
			if parseErr != nil {
				err = parseErr
				break
			}
			user, err = formatUUID(raw)
			if err != nil {
				break
			}
		}
		options = append(options, map[string]string{
			"id": employeeID, "employeeCode": code, "name": name, "employmentStatus": status,
			"officialEmail": email, "userId": user,
		})
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"employees": options})
}

func decodeAccess(w http.ResponseWriter, r *http.Request, dest any) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			writeError(w, http.StatusForbidden, "invalid_origin")
			return false
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func accessAudit(ctx context.Context, tx *sql.Tx, actor []byte, table, id string, old, updated any) error {
	var previous any
	if old != nil {
		raw, err := json.Marshal(old)
		if err != nil {
			return err
		}
		previous = string(raw)
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_id,database_actor,event,entity_table,entity_key,old_values,new_values)
		VALUES(?,CURRENT_USER(),?,?,?,CAST(? AS JSON),CAST(? AS JSON))`, actor, auditEvent(old), table, id, previous, string(raw))
	return err
}

func auditEvent(old any) string {
	if old == nil {
		return "create"
	}
	return "update"
}

func validUser(input *userInput, update bool) bool {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.LoginEmail = strings.TrimSpace(input.LoginEmail)
	input.Remarks = strings.TrimSpace(input.Remarks)
	if input.Username == "" || len(input.Username) > 191 || strings.Contains(input.Username, "@") || input.DisplayName == "" || len(input.DisplayName) > 191 {
		return false
	}
	if input.LoginEmail != "" && (!validEmail(input.LoginEmail) || len(input.LoginEmail) > 254) {
		return false
	}
	if input.LoginMethod == "email" && input.LoginEmail == "" {
		return false
	}
	if input.AccountStatus != "Active" && input.AccountStatus != "Inactive" && input.AccountStatus != "Locked" {
		return false
	}
	if input.LoginMethod != "username" && input.LoginMethod != "email" {
		return false
	}
	var roleErr error
	input.RoleIDs, roleErr = normalizeRoleIDs(input.RoleIDs)
	if roleErr != nil {
		return false
	}
	employeeID, err := parseUUID(input.EmployeeID)
	if err != nil {
		return false
	}
	input.EmployeeID, _ = formatUUID(employeeID)
	if len(input.Remarks) > 1000 {
		return false
	}
	if update {
		return input.Version > 0 && input.Password == ""
	}
	return input.Version == 0 && utf8.RuneCountInString(input.Password) >= 6 && len(input.Password) <= 72
}

func (h *Handler) saveUser(w http.ResponseWriter, r *http.Request) {
	var input userInput
	if !decodeAccess(w, r, &input) {
		return
	}
	update := r.Method == http.MethodPatch
	if !validUser(&input, update) {
		writeError(w, http.StatusBadRequest, "invalid_user_details_or_password_policy")
		return
	}
	id := r.PathValue("id")
	if !update {
		id, _ = newUUID()
	}
	userID, err := parseUUID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return
	}
	id, _ = formatUUID(userID)
	var passwordHash []byte
	if !update {
		passwordHash, err = bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusBadRequest, "password_policy")
			return
		}
	}
	db, ctx, cancel := h.accessDB(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	actor, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if update && id == actor.ID {
		writeError(w, http.StatusForbidden, "cannot_edit_your_own_account")
		return
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var old any
	var previousEmployee []byte
	if update {
		var stored User
		err = tx.QueryRowContext(ctx, `SELECT employee_id,username,login_method,display_name,COALESCE(login_email,''),account_status,version FROM users WHERE id=? AND deleted_at IS NULL FOR UPDATE`, userID).
			Scan(&previousEmployee, &stored.Username, &stored.LoginMethod, &stored.DisplayName, &stored.LoginEmail, &stored.AccountStatus, &stored.Version)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
			return
		}
		if stored.Version != input.Version {
			writeError(w, http.StatusConflict, "record_changed_reload_and_retry")
			return
		}
		stored.RoleIDs, err = readUserRoleIDs(ctx, tx, userID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "master_service_unavailable")
			return
		}
		stored.ID = id
		stored.EmployeeID, _ = formatUUID(previousEmployee)
		old = stored
	}
	employeeID, _ := parseUUID(input.EmployeeID)
	var employeeName, employeeCode, employmentStatus string
	err = tx.QueryRowContext(ctx, `SELECT name,employee_code,employment_status FROM employees WHERE id=? AND deleted_at IS NULL FOR UPDATE`, employeeID).
		Scan(&employeeName, &employeeCode, &employmentStatus)
	if err != nil {
		writeError(w, http.StatusBadRequest, "employee_not_available")
		return
	}
	if input.AccountStatus == "Active" && employmentStatus != "Active" {
		writeError(w, http.StatusBadRequest, "active_account_requires_active_employee")
		return
	}
	if update {
		_, err = tx.ExecContext(ctx, `UPDATE users SET employee_id=?,username=?,login_method=?,display_name=?,login_email=?,account_status=?,active=?,version=version+1,updated_by=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND version=?`,
			employeeID, input.Username, input.LoginMethod, input.DisplayName, nullableString(input.LoginEmail), input.AccountStatus, input.AccountStatus == "Active", actorID, userID, input.Version)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO users(id,employee_id,username,login_method,display_name,login_email,password_hash,account_status,active,password_reset_required,version,created_by,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,TRUE,1,?,?)`, userID, employeeID, input.Username, input.LoginMethod, input.DisplayName, nullableString(input.LoginEmail), string(passwordHash), input.AccountStatus, input.AccountStatus == "Active", actorID, actorID)
	}
	if err != nil {
		writeError(w, http.StatusConflict, "username_email_or_employee_already_in_use")
		return
	}
	if update {
		if _, err = tx.ExecContext(ctx, `UPDATE employees SET user_id=NULL WHERE user_id=?`, userID); err != nil {
			writeError(w, http.StatusConflict, "employee_mapping_conflict")
			return
		}
	}
	mapping, err := tx.ExecContext(ctx, `UPDATE employees SET user_id=? WHERE id=? AND (user_id IS NULL OR user_id=?)`, userID, employeeID, userID)
	if err != nil {
		writeError(w, http.StatusConflict, "employee_mapping_conflict")
		return
	}
	if affected, err := mapping.RowsAffected(); err != nil || affected != 1 {
		writeError(w, http.StatusConflict, "employee_mapping_conflict")
		return
	}
	if err = replaceUserRoles(ctx, tx, userID, employeeID, actorID, input.RoleIDs, input.Remarks); err != nil {
		switch {
		case errors.Is(err, errRoleNotFound):
			writeError(w, http.StatusBadRequest, "role_not_available")
		case errors.Is(err, errRoleReasonRequired):
			writeError(w, http.StatusBadRequest, "role_change_requires_reason")
		default:
			writeError(w, http.StatusServiceUnavailable, "role_assignment_save_failed")
		}
		return
	}
	if update {
		if _, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=UTC_TIMESTAMP(6) WHERE user_id=? AND revoked_at IS NULL`, userID); err != nil {
			writeError(w, http.StatusServiceUnavailable, "session_revocation_failed")
			return
		}
	}
	version := 1
	if update {
		version = input.Version + 1
	}
	saved := User{
		ID: id, EmployeeID: input.EmployeeID, EmployeeName: employeeName, EmployeeCode: employeeCode,
		Username: input.Username, LoginMethod: input.LoginMethod, DisplayName: input.DisplayName, LoginEmail: input.LoginEmail,
		AccountStatus: input.AccountStatus, RoleIDs: input.RoleIDs, Version: version,
	}
	if err = accessAudit(ctx, tx, actorID, "users", id, old, map[string]any{"user": saved, "remarks": input.Remarks}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "master_audit_failed")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "user_save_failed")
		return
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"user": saved})
}
