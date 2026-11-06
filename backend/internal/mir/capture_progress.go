package mir

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/mir/decoder"
)

// UTC preserves the existing date convention unless a business timezone has
// explicitly been configured. IANA tzdata is embedded for Windows deployments.
func businessLocation() (*time.Location, error) {
	zone := strings.TrimSpace(os.Getenv("BUSINESS_TIMEZONE"))
	if zone == "" {
		zone = "UTC"
	}
	return time.LoadLocation(zone)
}
func importedBounds(from, to string, location *time.Location) (time.Time, time.Time, error) {
	first, err := time.ParseInLocation("2006-01-02", from, location)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	last, err := time.ParseInLocation("2006-01-02", to, location)
	if err != nil || last.Before(first) {
		return time.Time{}, time.Time{}, errors.New("invalid_imported_date_range")
	}
	// AddDate follows local midnight across daylight-saving transitions.
	return first.UTC(), last.AddDate(0, 0, 1).UTC(), nil
}
func (h *StageHandler) captureSettings(w http.ResponseWriter, r *http.Request) {
	location, err := businessLocation()
	if err != nil {
		failure(w, 503, "The business timezone is invalid. Ask your administrator to check BUSINESS_TIMEZONE.")
		return
	}
	respond(w, 200, map[string]any{"timezone": location.String(), "today": time.Now().In(location).Format("2006-01-02"), "sales_save_enabled": false, "workflow_note": "Review the draft and complete customer and supplier accounting. Saving, approval and posting are not connected yet."})
}

type LinkedSale struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	Status string `json:"status"`
}
type CaptureProgress struct {
	Status          string      `json:"status"`
	Issues          []string    `json:"issues"`
	LinkedSale      *LinkedSale `json:"linked_sale"`
	CanPrepareDraft bool        `json:"can_prepare_draft"`
	CanViewSale     bool        `json:"can_view_sale"`
}

var canonicalTicket = regexp.MustCompile(`[^A-Z0-9]`)

func ticketKey(number string) string {
	return canonicalTicket.ReplaceAllString(strings.ToUpper(number), "")
}

// Existing sales_lines.ticket_original_sale_uq and tickets_ticket_number_key_uq
// remain the write-time guarantees. Never bypass them or clear a linked ticket
// when a document is cancelled, reversed or soft deleted.
type captureQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func linkedSales(ctx context.Context, db captureQueryer, numbers []string) (map[string]LinkedSale, error) {
	result := map[string]LinkedSale{}
	args := []any{}
	placeholders := []string{}
	seen := map[string]bool{}
	for _, number := range numbers {
		key := ticketKey(number)
		if key != "" && !seen[key] {
			seen[key] = true
			args = append(args, key)
			placeholders = append(placeholders, "?")
		}
	}
	if len(args) == 0 {
		return result, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT t.ticket_number_key,LOWER(HEX(d.id)),COALESCE(d.document_number,''),d.status FROM tickets t JOIN sales_lines l ON l.ticket_id=t.id JOIN sales_documents d ON d.id=l.document_id WHERE t.ticket_number_key IN (`+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var sale LinkedSale
		if err = rows.Scan(&key, &sale.ID, &sale.Number, &sale.Status); err != nil {
			return nil, err
		}
		result[key] = sale
	}
	return result, rows.Err()
}

func sourceProgress(row CaptureRow, status string, issues []decoder.Issue) CaptureProgress {
	progress := CaptureProgress{Status: "Needs Review", Issues: []string{}, CanPrepareDraft: status != "failed"}
	for _, issue := range issues {
		if issue.Severity != "info" {
			progress.Issues = append(progress.Issues, issue.Message)
		}
	}
	if status == "failed" {
		progress.Issues = append(progress.Issues, "Extraction failed. Review the MIR import errors before preparing a sale.")
	}
	fields := []struct{ value, label string }{{row.TicketNumber, "Ticket number"}, {row.PassengerName, "Passenger name"}, {row.PNR, "PNR"}, {row.IssueDate, "Valid ticket issue date"}, {row.Sector, "Linked itinerary"}, {row.Fare, "Unambiguous imported fare"}, {row.Currency, "Fare currency"}}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			progress.Issues = append(progress.Issues, field.label+" is missing.")
		}
	}
	if row.TicketNumber != "" && (len(ticketKey(row.TicketNumber)) != 13 || !draftDigits.MatchString(ticketKey(row.TicketNumber))) {
		progress.Issues = append(progress.Issues, "Review the ticket number; a 13-digit airline ticket number is required.")
	}
	if row.Fare != "" && !draftDecimal.MatchString(row.Fare) {
		progress.Issues = append(progress.Issues, "Imported fare has an unsupported amount format.")
	}
	if status == "review" && len(progress.Issues) == 0 {
		progress.Issues = append(progress.Issues, "The import requires validation review.")
	}
	if len(progress.Issues) == 0 {
		progress.Status = "Ready for Sale"
	}
	return progress
}

func (repo Repository) enrichCapture(ctx context.Context, rows []CaptureRow) error {
	if len(rows) == 0 {
		return nil
	}
	numbers := []string{}
	for _, row := range rows {
		numbers = append(numbers, row.TicketNumber)
	}
	sales, err := linkedSales(ctx, repo.DB, numbers)
	if err != nil {
		return err
	}
	currencies := map[string]bool{}
	masters, err := repo.DB.QueryContext(ctx, `SELECT code FROM currencies WHERE active=TRUE AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for masters.Next() {
		var code string
		if err = masters.Scan(&code); err != nil {
			break
		}
		currencies[code] = true
	}
	readErr := masters.Err()
	masters.Close()
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	airlines := map[string]int{}
	masters, err = repo.DB.QueryContext(ctx, `SELECT code,name FROM airlines WHERE active=TRUE AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for masters.Next() {
		var code, name string
		if err = masters.Scan(&code, &name); err != nil {
			break
		}
		code = strings.ToLower(strings.TrimSpace(code))
		name = strings.ToLower(strings.TrimSpace(name))
		airlines[code]++
		if name != code {
			airlines[name]++
		}
	}
	readErr = masters.Err()
	masters.Close()
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	for i := range rows {
		var issues []decoder.Issue
		if err := json.Unmarshal(rows[i].validationJSON, &issues); err != nil {
			return err
		}
		progress := sourceProgress(rows[i], rows[i].importStatus, issues)
		if !currencies[rows[i].Currency] {
			progress.Issues = append(progress.Issues, "Map the fare currency to an active currency in Entry.")
		}
		if airlines[strings.ToLower(strings.TrimSpace(rows[i].airlineSource))] != 1 {
			progress.Issues = append(progress.Issues, "Map the airline to an active airline in Entry.")
		}
		if len(progress.Issues) > 0 {
			progress.Status = "Needs Review"
		}
		if sale, ok := sales[ticketKey(rows[i].TicketNumber)]; ok {
			progress.Status = "Sale Created"
			progress.LinkedSale = &sale
			progress.CanPrepareDraft = false
		}
		rows[i].Progress = progress
	}
	return nil
}

func (repo Repository) canViewSales(ctx context.Context, userID string) (bool, error) {
	// No permissive role defaults: the grant must exist on an active assigned role.
	compact := strings.ReplaceAll(userID, "-", "")
	var allowed bool
	err := repo.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles role ON role.id=ur.role_id JOIN role_permissions rp ON rp.role_id=role.id JOIN permissions permission ON permission.id=rp.permission_id WHERE ur.user_id=UNHEX(?) AND role.active=TRUE AND role.deleted_at IS NULL AND permission.module='sales' AND permission.screen='workspace' AND permission.action='view')`, compact).Scan(&allowed)
	return allowed, err
}
func (h *StageHandler) captureSale(w http.ResponseWriter, r *http.Request) {
	passengerID, err := parseID(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "Invalid passenger ID.")
		return
	}
	recordID, err := parseID(r.URL.Query().Get("record_id"))
	if err != nil || len(r.URL.Query()["record_id"]) != 1 {
		failure(w, 400, "Invalid record ID.")
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	actor, _ := auth.PrincipalFromContext(r.Context())
	allowed, err := repo.canViewSales(ctx, actor.ID)
	if err != nil || !allowed {
		failure(w, 403, "Your role does not have permission to view sales records.")
		return
	}
	var number string
	if err := repo.DB.QueryRowContext(ctx, `SELECT ticket_number FROM mir_passenger WHERE id=? AND record_id=?`, passengerID, recordID).Scan(&number); err != nil {
		failure(w, 404, "Ticket no longer available.")
		return
	}
	sales, err := linkedSales(ctx, repo.DB, []string{number})
	if err != nil {
		failure(w, 503, "Linked sale could not be loaded.")
		return
	}
	sale, ok := sales[ticketKey(number)]
	if !ok {
		failure(w, 404, "No sale is linked to this ticket.")
		return
	}
	var date, currency, customer, narration string
	err = repo.DB.QueryRowContext(ctx, `SELECT DATE_FORMAT(d.document_date,'%Y-%m-%d'),d.currency_code,COALESCE(p.legal_name,''),COALESCE(d.narration,'') FROM sales_documents d LEFT JOIN customers c ON c.id=d.customer_id LEFT JOIN parties p ON p.id=c.party_id WHERE d.id=UNHEX(?)`, sale.ID).Scan(&date, &currency, &customer, &narration)
	if err != nil {
		failure(w, 503, "Linked sale details could not be loaded.")
		return
	}
	respond(w, 200, map[string]any{"sale": sale, "document_date": date, "currency": currency, "customer": customer, "narration": narration, "ticket_number": number})
}
