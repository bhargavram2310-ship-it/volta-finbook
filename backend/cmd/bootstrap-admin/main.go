package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"voltafinbook/backend/internal/database"
)

const (
	defaultUsername     = "SuperAdmin"
	defaultPassword     = "SuperAdmin"
	defaultEmployee     = "SuperAdmin"
	defaultEmployeeCode = "SUPERADMIN"
	defaultEmail        = "superadmin@volta.local"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("password hashing failed")
	}
	config, err := database.LoadConfig()
	if err != nil {
		return errors.New("MySQL database configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, config)
	if err != nil {
		return errors.New("could not connect to MySQL")
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.New("could not reserve a MySQL connection for administrator setup")
	}
	defer conn.Close()
	var lockResult sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('finbook_initial_admin', 10)`).Scan(&lockResult); err != nil || !lockResult.Valid || lockResult.Int64 != 1 {
		return errors.New("could not acquire the initial administrator setup lock")
	}
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer releaseCancel()
		var released sql.NullInt64
		if err := conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK('finbook_initial_admin')`).Scan(&released); err != nil || !released.Valid || released.Int64 != 1 {
			_ = conn.Close()
		}
	}()

	var schemaExists bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM information_schema.tables
		WHERE table_schema=DATABASE() AND table_name='employee_change_requests'
	)`).Scan(&schemaExists); err != nil || !schemaExists {
		return errors.New("MySQL application schema is not installed")
	}
	var loginMethodColumn bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM information_schema.columns
		WHERE table_schema=DATABASE() AND table_name='users' AND column_name='login_method'
	)`).Scan(&loginMethodColumn); err != nil {
		return errors.New("could not inspect MySQL user login-method schema")
	}
	if !loginMethodColumn {
		return errors.New("MySQL user login-method migration is not installed")
	}
	var existingUsers int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&existingUsers); err != nil {
		return errors.New("could not inspect administrator setup state")
	}
	if existingUsers != 0 {
		return errors.New("initial administrator setup is one-time and users already exist")
	}

	employeeID, err := newUUID()
	if err != nil {
		return err
	}
	userID, err := newUUID()
	if err != nil {
		return err
	}
	employeeIDBytes, err := uuidBytes(employeeID)
	if err != nil {
		return err
	}
	userIDBytes, err := uuidBytes(userID)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("could not begin administrator setup")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO employees
		(id,employee_code,name,official_email,employment_status,active,version)
		VALUES(?,?,?,?, 'Active', TRUE, 1)`, employeeIDBytes, defaultEmployeeCode, defaultEmployee, defaultEmail); err != nil {
		return errors.New("could not create administrator employee record")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users
		(id,employee_id,username,login_method,display_name,login_email,password_hash,password_reset_required,account_status,active,version)
		VALUES(?, ?, ?, 'username', ?, ?, ?, TRUE, 'Active', TRUE, 1)`, userIDBytes, employeeIDBytes, defaultUsername, defaultEmployee, defaultEmail, string(passwordHash)); err != nil {
		return errors.New("could not create administrator user")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET user_id=? WHERE id=?`, userIDBytes, employeeIDBytes); err != nil {
		return errors.New("could not link administrator employee and user")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("administrator setup failed and was rolled back")
	}
	fmt.Println("Created the initial SuperAdmin user and employee account.")
	fmt.Printf("Initial login: username %s, password %s. Change this password immediately after signing in.\n", defaultUsername, defaultPassword)
	return nil
}

func uuidBytes(value string) ([]byte, error) {
	encoded := strings.ReplaceAll(value, "-", "")
	result, err := hex.DecodeString(encoded)
	if err != nil || len(result) != 16 {
		return nil, errors.New("invalid administrator identifier")
	}
	return result, nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("could not generate administrator identifiers")
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}

func equalPassword(first, second []byte) bool {
	if len(first) != len(second) {
		return false
	}
	var difference byte
	for index := range first {
		difference |= first[index] ^ second[index]
	}
	return difference == 0
}
