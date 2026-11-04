package employees

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"voltafinbook/backend/internal/auth"
)

var (
	errRoleNotFound       = errors.New("role not found")
	errRoleReasonRequired = errors.New("role change reason required")
	roleCodePattern       = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type Role struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Version     int    `json:"version"`
}

type roleInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Remarks     string `json:"remarks"`
	Version     int    `json:"version,omitempty"`
}

func validRoleInput(input *roleInput, update bool) bool {
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Remarks = strings.TrimSpace(input.Remarks)
	return roleCodePattern.MatchString(input.Code) && len(input.Code) <= 100 &&
		input.Name != "" && len(input.Name) <= 191 && len(input.Description) <= 4000 &&
		len(input.Remarks) <= 1000 && ((update && input.Version > 0) || (!update && input.Version == 0))
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	db, ctx, cancel := h.accessDB(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id,code,name,COALESCE(description,''),active,version
		FROM roles WHERE deleted_at IS NULL ORDER BY name,code`)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "role_service_unavailable")
		return
	}
	defer rows.Close()
	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		var id []byte
		if err := rows.Scan(&id, &role.Code, &role.Name, &role.Description, &role.Active, &role.Version); err != nil {
			writeError(w, http.StatusServiceUnavailable, "role_service_unavailable")
			return
		}
		role.ID, err = formatUUID(id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "role_service_unavailable")
			return
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "role_service_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) saveRole(w http.ResponseWriter, r *http.Request) {
	var input roleInput
	if !decodeAccess(w, r, &input) {
		return
	}
	update := r.Method == http.MethodPatch
	if !validRoleInput(&input, update) {
		writeError(w, http.StatusBadRequest, "invalid_role_details")
		return
	}
	id := r.PathValue("id")
	if !update {
		var err error
		id, err = newUUID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "role_save_failed")
			return
		}
	}
	roleID, err := parseUUID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_role_id")
		return
	}
	id, _ = formatUUID(roleID)
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
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "role_save_failed")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var previous any
	if update {
		var old Role
		var oldID []byte
		err = tx.QueryRowContext(ctx, `SELECT id,code,name,COALESCE(description,''),active,version
			FROM roles WHERE id=? AND deleted_at IS NULL FOR UPDATE`, roleID).
			Scan(&oldID, &old.Code, &old.Name, &old.Description, &old.Active, &old.Version)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "role_save_failed")
			return
		}
		if old.Version != input.Version {
			writeError(w, http.StatusConflict, "record_changed_reload_and_retry")
			return
		}
		old.ID, err = formatUUID(oldID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "role_save_failed")
			return
		}
		previous = old
		_, err = tx.ExecContext(ctx, `UPDATE roles SET code=?,name=?,description=?,active=?,version=version+1,
			updated_by=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND version=?`,
			input.Code, input.Name, nullableString(input.Description), input.Active, actorID, roleID, input.Version)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO roles(id,code,name,description,active,version,created_by,updated_by)
			VALUES(?,?,?,?,?,1,?,?)`, roleID, input.Code, input.Name, nullableString(input.Description), input.Active, actorID, actorID)
	}
	if err != nil {
		writeError(w, http.StatusConflict, "role_code_already_in_use")
		return
	}
	version := 1
	if update {
		version = input.Version + 1
	}
	saved := Role{ID: id, Code: input.Code, Name: input.Name, Description: input.Description, Active: input.Active, Version: version}
	if err := accessAudit(ctx, tx, actorID, "roles", id, previous, map[string]any{"role": saved, "remarks": input.Remarks}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "role_audit_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "role_save_failed")
		return
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"role": saved})
}

func normalizeRoleIDs(values []string) ([]string, error) {
	if values == nil {
		return nil, errors.New("role list is required")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		id, err := parseUUID(value)
		if err != nil {
			return nil, err
		}
		normalized, err := formatUUID(id)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			return nil, errors.New("duplicate role")
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func parseRoleIDList(value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		id, err := hex.DecodeString(part)
		if err != nil {
			return nil, err
		}
		formatted, err := formatUUID(id)
		if err != nil {
			return nil, err
		}
		result = append(result, formatted)
	}
	return result, nil
}

func readUserRoleIDs(ctx context.Context, tx *sql.Tx, userID []byte) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT role_id FROM user_roles WHERE user_id=? ORDER BY role_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]string, 0)
	for rows.Next() {
		var id []byte
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		formatted, err := formatUUID(id)
		if err != nil {
			return nil, err
		}
		roles = append(roles, formatted)
	}
	return roles, rows.Err()
}

func replaceUserRoles(ctx context.Context, tx *sql.Tx, userID, employeeID, actorID []byte, roleIDs []string, remarks string) error {
	type assignedRole struct {
		id   []byte
		name string
	}
	current := make(map[string]assignedRole)
	rows, err := tx.QueryContext(ctx, `SELECT ur.role_id,r.name FROM user_roles ur
		JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=? FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var role assignedRole
		if err := rows.Scan(&role.id, &role.name); err != nil {
			rows.Close()
			return err
		}
		id, err := formatUUID(role.id)
		if err != nil {
			rows.Close()
			return err
		}
		current[id] = role
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	next := make(map[string]assignedRole, len(roleIDs))
	for _, value := range roleIDs {
		id, err := parseUUID(value)
		if err != nil {
			return errRoleNotFound
		}
		var name string
		err = tx.QueryRowContext(ctx, `SELECT name FROM roles WHERE id=? AND deleted_at IS NULL`, id).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return errRoleNotFound
		}
		if err != nil {
			return err
		}
		next[value] = assignedRole{id: id, name: name}
	}
	removed, added := make([]assignedRole, 0), make([]assignedRole, 0)
	for id, role := range current {
		if _, remains := next[id]; !remains {
			removed = append(removed, role)
		}
	}
	for id, role := range next {
		if _, existed := current[id]; !existed {
			added = append(added, role)
		}
	}
	if len(removed)+len(added) == 0 {
		return nil
	}
	remarks = strings.TrimSpace(remarks)
	if remarks == "" {
		return errRoleReasonRequired
	}
	for _, role := range removed {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=? AND role_id=?`, userID, role.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_history
			(user_id,employee_id,previous_role_id,previous_role_name,action,modified_by,remarks)
			VALUES(?,?,?,?,'Removed',?,?)`, userID, employeeID, role.id, role.name, actorID, remarks); err != nil {
			return err
		}
	}
	for _, role := range added {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id,created_by) VALUES(?,?,?)`, userID, role.id, actorID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_history
			(user_id,employee_id,new_role_id,new_role_name,action,modified_by,remarks)
			VALUES(?,?,?,?,'Assigned',?,?)`, userID, employeeID, role.id, role.name, actorID, remarks); err != nil {
			return err
		}
	}
	return nil
}
