package masters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mysql "github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"
)

type Record struct {
	ID       string            `json:"id"`
	Revision string            `json:"revision"`
	Values   map[string]string `json:"values"`
}
type mutation struct {
	Values   map[string]string `json:"values"`
	Revision string            `json:"revision"`
}
type Handler struct{ config *mysql.Config }

func NewHandler(config *mysql.Config) http.Handler {
	h := &Handler{config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/masters/{kind}", h.list)
	mux.HandleFunc("POST /api/v1/masters/{kind}", h.mutate)
	mux.HandleFunc("PATCH /api/v1/masters/{kind}/{id}", h.mutate)
	mux.HandleFunc("DELETE /api/v1/masters/{kind}/{id}", h.mutate)
	return auth.RequireAuthenticated(config, mux)
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func (h *Handler) open(w http.ResponseWriter, r *http.Request) (*sql.DB, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	db, err := database.Connect(ctx, h.config)
	if err != nil {
		cancel()
		failure(w, 503, "Master service unavailable. Check the database connection.")
		return nil, nil, nil
	}
	return db, ctx, cancel
}
func key(d Definition) string {
	if d.Key != "" {
		return d.Key
	}
	return "id"
}
func idExpr(d Definition) string {
	if d.Key != "" {
		return "t.`" + d.Key + "`"
	}
	return "LOWER(HEX(t.id))"
}
func source(d Definition) string {
	s := "`" + d.Table + "` t"
	if d.Party {
		s += " JOIN parties p ON p.id=t.party_id"
	}
	return s
}
func predicate(d Definition) string {
	if d.Party {
		return "p.deleted_at IS NULL"
	}
	if d.NoSoftDelete {
		return "1=1"
	}
	return "t.deleted_at IS NULL"
}
func column(d Definition, f Field) string {
	prefix := "t."
	if d.Party && partyColumns[f.Name] {
		prefix = "p."
	}
	return prefix + "`" + f.Name + "`"
}
func isUUID(f Field) bool { return f.Type == "reference" && catalog[f.Ref].Key == "" }
func selectSQL(d Definition) string {
	columns := []string{idExpr(d)}
	for _, f := range d.Fields {
		c := column(d, f)
		if isUUID(f) {
			c = "LOWER(HEX(" + c + "))"
		}
		if f.Type == "date" {
			c = "DATE_FORMAT(" + c + ",'%Y-%m-%d')"
		}
		columns = append(columns, c)
	}
	return "SELECT " + strings.Join(columns, ",") + " FROM " + source(d) + " WHERE " + predicate(d)
}

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner, d Definition) (Record, error) {
	var id string
	values := make([]sql.NullString, len(d.Fields))
	args := []any{&id}
	for i := range values {
		args = append(args, &values[i])
	}
	if err := row.Scan(args...); err != nil {
		return Record{}, err
	}
	record := Record{ID: id, Values: map[string]string{}}
	for i, f := range d.Fields {
		value := values[i].String
		if f.Type == "decimal" || f.Type == "rate" || f.Type == "percent" {
			if strings.Contains(value, ".") {
				value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
			}
		}
		if f.Type == "boolean" {
			if value == "1" {
				value = "true"
			} else {
				value = "false"
			}
		}
		record.Values[f.Name] = value
	}
	raw, _ := json.Marshal(record.Values)
	record.Revision = fmt.Sprintf("%x", sha256.Sum256(raw))
	return record, nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	d, ok := catalog[r.PathValue("kind")]
	if !ok {
		failure(w, 404, "Unknown master.")
		return
	}
	db, ctx, cancel := h.open(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	rows, err := db.QueryContext(ctx, selectSQL(d)+" ORDER BY "+idExpr(d))
	if err != nil {
		failure(w, 503, "Master records could not be loaded.")
		return
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		record, e := scanRecord(rows, d)
		if e != nil {
			failure(w, 503, "Master records could not be read.")
			return
		}
		records = append(records, record)
	}
	if rows.Err() != nil {
		failure(w, 503, "Master records could not be read.")
		return
	}
	respond(w, 200, map[string]any{"title": d.Title, "fields": d.Fields, "records": records})
}

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)
var decimalValue = regexp.MustCompile(`^\d{1,18}(\.\d{1,12})?$`)

func normalize(d Definition, input *mutation) error {
	known := map[string]bool{}
	for _, f := range d.Fields {
		known[f.Name] = true
		value := strings.TrimSpace(input.Values[f.Name])
		input.Values[f.Name] = value
		if value == "" {
			if f.Required {
				return fmt.Errorf("%s is required.", f.Label)
			}
			continue
		}
		if f.Max > 0 && utf8.RuneCountInString(value) > f.Max {
			return fmt.Errorf("%s is too long.", f.Label)
		}
		valid := true
		switch f.Type {
		case "boolean":
			valid = value == "true" || value == "false"
		case "currency":
			valid = currencyCode.MatchString(value)
		case "select":
			valid = false
			for _, option := range f.Options {
				if value == option {
					valid = true
				}
			}
		case "reference":
			if isUUID(f) {
				valid = hexID.MatchString(value)
			} else {
				valid = currencyCode.MatchString(value)
			}
		case "date":
			_, err := time.Parse("2006-01-02", value)
			valid = err == nil
		case "json":
			var obj map[string]any
			err := json.Unmarshal([]byte(value), &obj)
			valid = err == nil && obj != nil
		case "integer", "precision":
			n, err := strconv.ParseInt(value, 10, 32)
			valid = err == nil && n >= 0
			if f.Name == "version" {
				valid = valid && n > 0
			}
			if f.Type == "precision" {
				valid = valid && n <= 6
			}
		case "decimal", "rate", "percent":
			valid = decimalValue.MatchString(value)
			parts := strings.Split(value, ".")
			if f.Type != "rate" && len(parts) > 1 && len(parts[1]) > 6 {
				valid = false
			}
			if f.Type == "rate" {
				valid = valid && strings.Trim(value, "0.") != ""
			}
			if f.Type == "percent" {
				whole, _ := strconv.ParseUint(parts[0], 10, 64)
				valid = valid && whole <= 100 && (whole != 100 || len(parts) == 1 || strings.Trim(parts[1], "0") == "")
			}
		}
		if !valid {
			return fmt.Errorf("%s is invalid.", f.Label)
		}
	}
	for name := range input.Values {
		if !known[name] {
			return errors.New("Unknown master field.")
		}
	}
	v := input.Values
	if v["starts_on"] != "" && v["ends_on"] < v["starts_on"] {
		return errors.New("End date must be on or after start date.")
	}
	if v["effective_to"] != "" && v["effective_to"] < v["effective_from"] {
		return errors.New("Effective end date must be on or after start date.")
	}
	if v["from_currency"] != "" && v["from_currency"] == v["to_currency"] {
		return errors.New("Choose two different currencies.")
	}
	if v["kind"] == "cash" && v["bank_name"] != "" {
		return errors.New("Cash accounts cannot have a bank name.")
	}
	return nil
}
func dbValue(f Field, value string) any {
	if value == "" {
		return nil
	}
	if isUUID(f) {
		raw, _ := hex.DecodeString(value)
		return raw
	}
	if f.Type == "boolean" {
		return value == "true"
	}
	return value
}
func rawID(d Definition, id string) (any, error) {
	if d.Key != "" {
		if !currencyCode.MatchString(id) {
			return nil, errors.New("Invalid currency code.")
		}
		return id, nil
	}
	if !hexID.MatchString(id) {
		return nil, errors.New("Invalid record ID.")
	}
	return hex.DecodeString(id)
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return hex.EncodeToString(b[:]), nil
}

func decodeMutation(w http.ResponseWriter, r *http.Request, input *mutation) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		failure(w, 403, "Invalid request origin.")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != r.Host {
			failure(w, 403, "Invalid request origin.")
			return false
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		failure(w, 400, "Invalid request.")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		failure(w, 400, "Invalid request.")
		return false
	}
	return true
}

func mutationError(w http.ResponseWriter, err error) {
	var dbErr *mysql.MySQLError
	if errors.As(err, &dbErr) {
		switch dbErr.Number {
		case 1062:
			failure(w, 409, "This code or configuration already exists.")
			return
		case 1451:
			failure(w, 409, "This record is in use and cannot be deleted. You can mark it inactive where available.")
			return
		case 1452:
			failure(w, 409, "A selected master no longer exists. Reload and select it again.")
			return
		case 3819, 1264, 1406:
			failure(w, 400, "These values do not meet the master rules. Check dates, amounts and required fields.")
			return
		}
	}
	failure(w, 503, "The master change could not be saved. Try again.")
}

func (h *Handler) mutate(w http.ResponseWriter, r *http.Request) {
	d, ok := catalog[r.PathValue("kind")]
	if !ok {
		failure(w, 404, "Unknown master.")
		return
	}
	var input mutation
	if !decodeMutation(w, r, &input) {
		return
	}
	deleting := r.Method == http.MethodDelete
	creating := r.Method == http.MethodPost
	if !deleting {
		if input.Values == nil {
			failure(w, 400, "Master values are required.")
			return
		}
		if err := normalize(d, &input); err != nil {
			failure(w, 400, err.Error())
			return
		}
	}
	id := r.PathValue("id")
	if creating {
		if d.Key != "" {
			id = input.Values[d.Key]
		} else {
			var err error
			id, err = newID()
			if err != nil {
				failure(w, 500, "Unable to generate record ID.")
				return
			}
		}
	}
	idValue, err := rawID(d, id)
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	db, ctx, cancel := h.open(w, r)
	if db == nil {
		return
	}
	defer cancel()
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		mutationError(w, err)
		return
	}
	defer tx.Rollback()
	var previous *Record
	if !creating {
		old, e := scanRecord(tx.QueryRowContext(ctx, selectSQL(d)+" AND t.`"+key(d)+"`=? FOR UPDATE", idValue), d)
		if errors.Is(e, sql.ErrNoRows) {
			failure(w, 404, "This master record no longer exists.")
			return
		}
		if e != nil {
			mutationError(w, e)
			return
		}
		previous = &old
		if input.Revision == "" || old.Revision != input.Revision {
			failure(w, 409, "This record changed. Reload before editing or deleting it.")
			return
		}
		for _, f := range d.Fields {
			if !deleting && f.Immutable && old.Values[f.Name] != input.Values[f.Name] {
				failure(w, 400, f.Label+" cannot be changed.")
				return
			}
		}
	}
	if !deleting {
		if err := validateRelations(ctx, tx, d, id, input.Values); err != nil {
			failure(w, 400, err.Error())
			return
		}
		if !creating && d.Table == "currencies" && previous.Values["decimal_places"] != input.Values["decimal_places"] {
			used, checkErr := currencyInUse(ctx, tx, id)
			if checkErr != nil {
				mutationError(w, checkErr)
				return
			}
			if used {
				failure(w, 409, "Currency precision cannot change after the currency is in use.")
				return
			}
		}
	}
	if deleting {
		if d.Table == "sales_types" {
			var used bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sales_documents WHERE sales_class=?)", previous.Values["sales_class"]).Scan(&used); err != nil {
				mutationError(w, err)
				return
			}
			if used {
				failure(w, 409, "This sales type is used by sales documents. Mark it inactive instead of deleting it.")
				return
			}
		}
		var partyID []byte
		if d.Party {
			err = tx.QueryRowContext(ctx, "SELECT party_id FROM `"+d.Table+"` WHERE id=?", idValue).Scan(&partyID)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM `"+d.Table+"` WHERE `"+key(d)+"`=?", idValue)
		}
		if err == nil && d.Party {
			var linked bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM customers WHERE party_id=?) OR EXISTS(SELECT 1 FROM suppliers WHERE party_id=?) OR EXISTS(SELECT 1 FROM travel_agents WHERE party_id=?)`, partyID, partyID, partyID).Scan(&linked)
			if err == nil && !linked {
				_, err = tx.ExecContext(ctx, "DELETE FROM parties WHERE id=?", partyID)
			}
		}
	} else {
		var partyID any
		if d.Party {
			if creating {
				pID, e := newID()
				if e != nil {
					mutationError(w, e)
					return
				}
				partyID, _ = hex.DecodeString(pID)
			} else {
				var pID []byte
				err = tx.QueryRowContext(ctx, "SELECT party_id FROM `"+d.Table+"` WHERE id=?", idValue).Scan(&pID)
				partyID = pID
			}
			if err == nil {
				err = saveFields(ctx, tx, d, "parties", partyID, input.Values, creating, true)
			}
		}
		if err == nil {
			if d.Party && creating {
				_, err = tx.ExecContext(ctx, "INSERT INTO `"+d.Table+"` (id,party_id) VALUES (?,?)", idValue, partyID)
				if err == nil {
					err = saveFields(ctx, tx, d, d.Table, idValue, input.Values, false, false)
				}
			} else {
				err = saveFields(ctx, tx, d, d.Table, idValue, input.Values, creating, false)
			}
		}
	}
	if err != nil {
		mutationError(w, err)
		return
	}
	var saved *Record
	if !deleting {
		result, e := scanRecord(tx.QueryRowContext(ctx, selectSQL(d)+" AND t.`"+key(d)+"`=?", idValue), d)
		if e != nil {
			mutationError(w, e)
			return
		}
		saved = &result
	}
	principal, _ := auth.PrincipalFromContext(ctx)
	actor, _ := hex.DecodeString(strings.ReplaceAll(principal.ID, "-", ""))
	event := "update"
	if creating {
		event = "create"
	}
	if deleting {
		event = "delete"
	}
	oldJSON, _ := json.Marshal(previous)
	newJSON, _ := json.Marshal(saved)
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_id,database_actor,event,entity_table,entity_key,old_values,new_values) VALUES(?,CURRENT_USER(),?,?,?,?,?)`, actor, event, d.Table, id, string(oldJSON), string(newJSON))
	if err != nil {
		mutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		mutationError(w, err)
		return
	}
	status := http.StatusOK
	if creating {
		status = http.StatusCreated
	}
	respond(w, status, map[string]any{"record": saved})
}

func saveFields(ctx context.Context, tx *sql.Tx, d Definition, table string, id any, values map[string]string, create, party bool) error {
	cols, marks, sets := []string{}, []string{}, []string{}
	args := []any{}
	for _, f := range d.Fields {
		if d.Party && partyColumns[f.Name] != party {
			continue
		}
		if !create && f.Immutable {
			continue
		}
		cols = append(cols, "`"+f.Name+"`")
		marks = append(marks, "?")
		sets = append(sets, "`"+f.Name+"`=?")
		args = append(args, dbValue(f, values[f.Name]))
	}
	if !party && (d.Table == "tax_rules" || d.Table == "commercial_rules") {
		sets = append(sets, "active=FALSE", "verification_status='to_be_verified_by_finance'")
		if d.Table == "tax_rules" {
			sets = append(sets, "verified_by=NULL", "verified_at=NULL")
		}
	}
	if !create && len(sets) == 0 {
		return nil
	}
	if create {
		if d.Key == "" || party {
			cols = append(cols, "id")
			marks = append(marks, "?")
			args = append(args, id)
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO `"+table+"` ("+strings.Join(cols, ",")+") VALUES ("+strings.Join(marks, ",")+")", args...)
		return err
	}
	if party || (!d.Party && d.Table != "currencies" && d.Table != "exchange_rates") {
		sets = append(sets, "updated_at=UTC_TIMESTAMP(6)")
	}
	args = append(args, id)
	k := key(d)
	if party {
		k = "id"
	}
	_, err := tx.ExecContext(ctx, "UPDATE `"+table+"` SET "+strings.Join(sets, ",")+" WHERE `"+k+"`=?", args...)
	return err
}

// Protect precision wherever the currency is referenced, including future tables.
func currencyInUse(ctx context.Context, tx *sql.Tx, code string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT TABLE_NAME,COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME='currencies' AND REFERENCED_COLUMN_NAME='code'`)
	if err != nil {
		return false, err
	}
	type target struct{ table, column string }
	targets := []target{}
	for rows.Next() {
		var item target
		if err = rows.Scan(&item.table, &item.column); err != nil {
			break
		}
		targets = append(targets, item)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if rowErr != nil {
		return false, rowErr
	}
	quote := func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
	for _, item := range targets {
		var used bool
		err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+quote(item.table)+" WHERE "+quote(item.column)+"=?)", code).Scan(&used)
		if err != nil {
			return false, err
		}
		if used {
			return true, nil
		}
	}
	return false, nil
}

func validateRelations(ctx context.Context, tx *sql.Tx, d Definition, id string, values map[string]string) error {
	for _, f := range d.Fields {
		value := values[f.Name]
		if f.Type != "reference" || value == "" {
			continue
		}
		target := catalog[f.Ref]
		raw, e := rawID(target, value)
		if e != nil {
			return e
		}
		if f.Name == "parent_id" && value == id {
			return errors.New("A record cannot be its own parent.")
		}
		var exists string
		e = tx.QueryRowContext(ctx, "SELECT "+idExpr(target)+" FROM "+source(target)+" WHERE "+predicate(target)+" AND t.`"+key(target)+"`=? FOR SHARE", raw).Scan(&exists)
		if e != nil {
			return fmt.Errorf("Select an existing %s.", f.Label)
		}
		if f.Name == "parent_id" {
			seen := map[string]bool{id: true}
			current := value
			for current != "" {
				if seen[current] {
					return errors.New("Parent relationships cannot form a cycle.")
				}
				seen[current] = true
				raw, _ := rawID(target, current)
				var parent sql.NullString
				if e := tx.QueryRowContext(ctx, "SELECT LOWER(HEX(parent_id)) FROM `"+target.Table+"` WHERE id=?", raw).Scan(&parent); e != nil {
					return errors.New("Parent record could not be checked.")
				}
				current = parent.String
			}
		}
	}
	return nil
}
