package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"voltafinbook/backend/internal/database"
)

const (
	sessionCookieName = "finbook_session"
	sessionLifetime   = 12 * time.Hour
	maxLoginBody      = 16 << 10
)

type Handler struct {
	config       *mysql.Config
	secureCookie bool
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type passwordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type user struct {
	ID              string       `json:"id"`
	EmployeeID      string       `json:"employeeId"`
	Username        string       `json:"username"`
	LoginMethod     string       `json:"loginMethod"`
	DisplayName     string       `json:"displayName"`
	LoginEmail      string       `json:"loginEmail"`
	AccountStatus   string       `json:"accountStatus"`
	PasswordChange  bool         `json:"passwordChangeRequired"`
	PasswordHash    string       `json:"-"`
	BlockedUntil    sql.NullTime `json:"-"`
	idBytes         []byte
	employeeIDBytes []byte
}

type contextKey struct{}

type Principal struct {
	ID                     string
	EmployeeID             string
	PasswordChangeRequired bool
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func RequireAuthenticated(config *mysql.Config, next http.Handler) http.Handler {
	handler := &Handler{config: config}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db, account, err := handler.authenticatedUser(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication_required")
			return
		}
		defer db.Close()
		if account.PasswordChange {
			writeError(w, http.StatusForbidden, "password_change_required")
			return
		}
		principal := Principal{ID: account.ID, EmployeeID: account.EmployeeID}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, principal)))
	})
}

func NewHandler(config *mysql.Config, secureCookie bool) http.Handler {
	handler := &Handler{config: config, secureCookie: secureCookie}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", handler.login)
	mux.HandleFunc("/api/v1/auth/me", handler.me)
	mux.HandleFunc("/api/v1/auth/logout", handler.logout)
	mux.HandleFunc("/api/v1/auth/password", handler.changePassword)
	return mux
}

func (handler *Handler) open(ctx context.Context) (*sql.DB, error) {
	if handler == nil || handler.config == nil {
		return nil, errors.New("database not configured")
	}
	return database.Connect(ctx, handler.config)
}

func (handler *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	var input loginRequest
	if !decodeBody(w, r, &input) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if username == "" || len(username) > 254 || len(input.Password) == 0 || len(input.Password) > 72 {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()

	var account user
	var accountID, employeeID []byte
	var loginEmail sql.NullString
	rows, err := tx.QueryContext(ctx, `SELECT id, employee_id, username, login_method, display_name, login_email,
		account_status, password_reset_required, password_hash, login_blocked_until
		FROM users
		WHERE ((login_method='username' AND LOWER(username)=?) OR (login_method='email' AND LOWER(login_email)=?)) AND deleted_at IS NULL
		ORDER BY id LIMIT 2 FOR UPDATE`, username, username)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	found := false
	ambiguous := false
	if rows.Next() {
		found = true
		err = rows.Scan(&accountID, &employeeID, &account.Username, &account.LoginMethod, &account.DisplayName,
			&loginEmail, &account.AccountStatus, &account.PasswordChange, &account.PasswordHash, &account.BlockedUntil)
	}
	if err == nil && rows.Next() {
		ambiguous = true
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if err != nil || rowsErr != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	if found {
		account.ID, err = formatUUID(accountID)
		if err == nil {
			account.EmployeeID, err = formatUUID(employeeID)
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
			return
		}
		account.idBytes = accountID
		account.employeeIDBytes = employeeID
		if loginEmail.Valid {
			account.LoginEmail = loginEmail.String
		}
	}

	hash := []byte(dummyPasswordHash)
	if found {
		hash = []byte(account.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(input.Password))
	blocked := account.BlockedUntil.Valid && account.BlockedUntil.Time.After(time.Now())
	if !found || ambiguous || passwordErr != nil || blocked || account.AccountStatus != "Active" || account.EmployeeID == "" {
		if found && !blocked && !ambiguous {
			if _, err := tx.ExecContext(ctx, `UPDATE users
				SET failed_login_count=LEAST(failed_login_count+1, 5),
				login_blocked_until=CASE WHEN failed_login_count>=4
					THEN DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 15 MINUTE)
					ELSE login_blocked_until END
				WHERE id=?`, account.idBytes); err != nil {
				writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}
			if err := tx.Commit(); err != nil {
				writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	var active bool
	err = tx.QueryRowContext(ctx, `SELECT employment_status='Active' AND deleted_at IS NULL
		FROM employees WHERE id=? FOR UPDATE`, account.employeeIDBytes).Scan(&active)
	if err != nil || !active {
		_ = tx.Rollback()
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authentication_unavailable")
		return
	}
	sessionID, err := newUUIDBytes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authentication_unavailable")
		return
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users
		SET failed_login_count=0, login_blocked_until=NULL, last_login_at=UTC_TIMESTAMP(6)
		WHERE id=?`, account.idBytes); err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sessions(id, user_id, token_hash, expires_at)
		VALUES(?, ?, ?, DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 12 HOUR))`, sessionID, account.idBytes, tokenHash[:]); err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	handler.clearCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/api/v1", HttpOnly: true,
		Secure: handler.secureCookie, SameSite: http.SameSiteStrictMode,
		MaxAge: int(sessionLifetime.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": account})
}

func (handler *Handler) me(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	db, account, err := handler.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	defer db.Close()
	writeJSON(w, http.StatusOK, map[string]any{"user": account})
}

func (handler *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if tokenHash, valid := hashSessionToken(cookie.Value); valid && handler.config != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			db, err := handler.open(ctx)
			if err != nil {
				handler.clearCookie(w)
				writeError(w, http.StatusServiceUnavailable, "logout_unavailable")
				return
			}
			_, err = db.ExecContext(ctx, `UPDATE sessions SET revoked_at=UTC_TIMESTAMP(6)
				WHERE token_hash=? AND revoked_at IS NULL`, tokenHash[:])
			_ = db.Close()
			if err != nil {
				handler.clearCookie(w)
				writeError(w, http.StatusServiceUnavailable, "logout_unavailable")
				return
			}
		}
	}
	handler.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	var input passwordRequest
	if !decodeBody(w, r, &input) {
		return
	}
	if !validNewPassword(input.NewPassword) {
		writeError(w, http.StatusBadRequest, "password_policy")
		return
	}
	db, account, err := handler.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	defer db.Close()
	var currentHash string
	if err := db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=? AND account_status='Active' AND deleted_at IS NULL`, account.idBytes).Scan(&currentHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials")
		} else {
			writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		}
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(input.CurrentPassword)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_current_password")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_update_failed")
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=?, password_reset_required=FALSE,
		failed_login_count=0, login_blocked_until=NULL WHERE id=?`, string(newHash), account.idBytes); err != nil {
		writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	tokenHash, valid := hashSessionToken(cookie.Value)
	if !valid {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=UTC_TIMESTAMP(6)
		WHERE user_id=? AND revoked_at IS NULL AND token_hash<>?`, account.idBytes, tokenHash[:]); err != nil {
		writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		return
	}
	newToken, newTokenHash, err := newSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_update_failed")
		return
	}
	sessionID, err := newUUIDBytes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_update_failed")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,expires_at)
		VALUES(?,?,?,DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 12 HOUR))`, sessionID, account.idBytes, newTokenHash[:]); err != nil {
		writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "password_update_failed")
		return
	}
	handler.clearCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: newToken, Path: "/api/v1", HttpOnly: true,
		Secure: handler.secureCookie, SameSite: http.SameSiteStrictMode,
		MaxAge: int(sessionLifetime.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
}

func validNewPassword(password string) bool {
	return utf8.RuneCountInString(password) >= 6 && len(password) <= 72
}

func (handler *Handler) authenticatedUser(r *http.Request) (*sql.DB, user, error) {
	var account user
	if handler == nil || handler.config == nil {
		return nil, account, errors.New("database not configured")
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, account, errors.New("session missing")
	}
	tokenHash, valid := hashSessionToken(cookie.Value)
	if !valid {
		return nil, account, errors.New("session invalid")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	db, err := handler.open(ctx)
	if err != nil {
		return nil, account, err
	}
	var accountID, employeeID []byte
	var loginEmail sql.NullString
	err = db.QueryRowContext(ctx, `SELECT u.id, u.employee_id, u.username, u.display_name,
		u.login_email, u.account_status, u.password_reset_required
		FROM sessions s INNER JOIN users u ON u.id=s.user_id
		INNER JOIN employees e ON e.id=u.employee_id
		WHERE s.token_hash=? AND s.revoked_at IS NULL AND s.expires_at>UTC_TIMESTAMP(6)
		AND u.account_status='Active' AND u.active=TRUE AND u.deleted_at IS NULL
		AND e.employment_status='Active' AND e.deleted_at IS NULL`, tokenHash[:]).Scan(
		&accountID, &employeeID, &account.Username, &account.DisplayName,
		&loginEmail, &account.AccountStatus, &account.PasswordChange,
	)
	if err != nil {
		_ = db.Close()
		return nil, user{}, err
	}
	account.ID, err = formatUUID(accountID)
	if err == nil {
		account.EmployeeID, err = formatUUID(employeeID)
	}
	if err != nil {
		_ = db.Close()
		return nil, user{}, err
	}
	account.idBytes = accountID
	account.employeeIDBytes = employeeID
	if loginEmail.Valid {
		account.LoginEmail = loginEmail.String
	}
	return db, account, nil
}

func (handler *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/api/v1", HttpOnly: true,
		Secure: handler.secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	// Retire cookies issued before sessions were shared with the master endpoints.
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/api/v1/auth", HttpOnly: true,
		Secure: handler.secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

func newSessionToken() (string, [32]byte, error) {
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	return token, sha256.Sum256([]byte(token)), nil
}

func newUUIDBytes() ([]byte, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return value, nil
}

func formatUUID(value []byte) (string, error) {
	if len(value) != 16 {
		return "", errors.New("invalid binary UUID")
	}
	encoded := hex.EncodeToString(value)
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}

func hashSessionToken(token string) ([32]byte, bool) {
	var empty [32]byte
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return empty, false
	}
	return sha256.Sum256([]byte(token)), true
}

func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	return false
}

func decodeBody(w http.ResponseWriter, r *http.Request, dest any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
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

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("unknown-user-timing-only"), bcrypt.DefaultCost)
	if err != nil {
		panic("failed to initialize authentication timing hash")
	}
	dummyPasswordHash = string(hash)
}

var dummyPasswordHash string
