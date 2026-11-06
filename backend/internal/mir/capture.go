package mir

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"voltafinbook/backend/internal/auth"
)

type CaptureRow struct {
	RecordID        int64           `json:"record_id"`
	PassengerID     int64           `json:"passenger_id"`
	TicketNumber    string          `json:"ticket_number"`
	PNR             string          `json:"pnr"`
	GDS             string          `json:"gds"`
	Staff           string          `json:"staff"`
	ClientReference string          `json:"client_reference"`
	InvoiceNumber   string          `json:"invoice_number"`
	Branch          string          `json:"branch"`
	PassengerName   string          `json:"passenger_name"`
	IssueDate       string          `json:"issue_date"`
	IssueDateSource string          `json:"issue_date_source"`
	Sector          string          `json:"sector"`
	WebReference    string          `json:"web_reference"`
	Fare            string          `json:"fare"`
	Currency        string          `json:"currency"`
	ProcessingDate  string          `json:"processing_date"`
	ImportedAt      string          `json:"imported_at"`
	Progress        CaptureProgress `json:"progress"`
	importStatus    string
	validationJSON  []byte
	airlineSource   string
}

func captureParams(q url.Values) (RecordFilter, string, error) {
	copy := url.Values{}
	for k, v := range q {
		if k != "date_basis" {
			copy[k] = v
		}
	}
	f, err := filterParams(copy)
	basis := q.Get("date_basis")
	if basis == "" {
		basis = "processing"
	}
	if err != nil {
		return f, basis, err
	}
	if len(q["date_basis"]) > 1 || (basis != "issue" && basis != "processing") {
		return f, basis, errors.New("invalid_capture_date_basis")
	}
	if f.From == "" || f.To == "" {
		return f, basis, errors.New("choose_from_and_to_dates")
	}
	return f, basis, nil
}

// A02DTE is a full source issue date. Validate its day against the month before
// using STR_TO_DATE; invalid source text remains visible without a guessed year.
const issueTextSQL = `UPPER(TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p.decoded_data,'$.A02DTE')),'')))`
const issueDateSQL = `(CASE WHEN ` + issueTextSQL + ` REGEXP '^[0-9]{2}(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)[0-9]{2}$'
 THEN CASE WHEN CAST(LEFT(` + issueTextSQL + `,2) AS UNSIGNED) BETWEEN 1 AND DAY(LAST_DAY(STR_TO_DATE(CONCAT('01',SUBSTRING(` + issueTextSQL + `,3)),'%d%b%y')))
 THEN STR_TO_DATE(` + issueTextSQL + `,'%d%b%y') END END)`

const captureJoin = ` FROM mir_passenger p JOIN mir_record r ON r.id=p.record_id JOIN mir_source_file s ON s.id=r.source_file_id `
const captureColumns = `r.id,p.id,p.ticket_number,r.locator,
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(r.decoded_header,'$.T50TRC')),''),
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(r.decoded_header,'$.T50SIN')),''),
 COALESCE((SELECT CASE WHEN COUNT(DISTINCT NULLIF(JSON_UNQUOTE(JSON_EXTRACT(pay.decoded_data,'$.A11CCH')),''))=1 THEN MAX(JSON_UNQUOTE(JSON_EXTRACT(pay.decoded_data,'$.A11CCH'))) ELSE '' END FROM mir_payment pay WHERE pay.record_id=r.id),''),
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p.decoded_data,'$.A02INV')),''),
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(r.decoded_header,'$.T50TPC')),''),
 COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(p.decoded_data,'$.A02EXN')),''),p.name),
 COALESCE(DATE_FORMAT(` + issueDateSQL + `,'%Y-%m-%d'),''),` + issueTextSQL + `,
 COALESCE((SELECT GROUP_CONCAT(CONCAT(seg.origin,'–',seg.destination) ORDER BY item.item_number,seg.id SEPARATOR ' / ')
 FROM mir_item_relationship link JOIN mir_segment seg ON seg.source_item_id=link.to_item_id
 JOIN mir_section_item item ON item.id=seg.source_item_id
 WHERE link.record_id=r.id AND link.from_item_id=p.source_item_id AND link.relationship_type='passenger_segment' AND seg.origin<>'' AND seg.destination<>''),''),
 COALESCE((SELECT CASE WHEN COUNT(*)=1 THEN MAX(f.total_amount) ELSE '' END FROM mir_item_relationship link JOIN mir_fare f ON f.source_item_id=link.to_item_id
 WHERE link.record_id=r.id AND link.from_item_id=p.source_item_id AND link.relationship_type='passenger_fare'),''),
 COALESCE((SELECT CASE WHEN COUNT(*)=1 THEN MAX(f.currency) ELSE '' END FROM mir_item_relationship link JOIN mir_fare f ON f.source_item_id=link.to_item_id
 WHERE link.record_id=r.id AND link.from_item_id=p.source_item_id AND link.relationship_type='passenger_fare'),''),
 DATE_FORMAT(r.imported_at,'%Y-%m-%d'),DATE_FORMAT(r.imported_at,'%Y-%m-%dT%H:%i:%sZ'),r.status,r.validation_issues,
 COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(r.decoded_header,'$.T50ISC')),''),JSON_UNQUOTE(JSON_EXTRACT(r.decoded_header,'$.T50ISN')),'')`

func (repo Repository) Capture(ctx context.Context, f RecordFilter, basis string) ([]CaptureRow, int, error) {
	from, to := f.From, f.To
	search, ticket, passenger := f.Search, f.Ticket, f.Passenger
	f.From = ""
	f.To = ""
	f.Search = ""
	f.Ticket = ""
	f.Passenger = ""
	where, args := f.where()
	if search != "" {
		prefix := escapeLike(search) + "%"
		where += " AND (r.locator LIKE ? OR s.filename LIKE ? OR p.ticket_number LIKE ? OR p.name LIKE ? OR JSON_UNQUOTE(JSON_EXTRACT(p.decoded_data,'$.A02EXN')) LIKE ?)"
		args = append(args, prefix, prefix, prefix, prefix, prefix)
	}
	if ticket != "" {
		where += " AND p.ticket_number=?"
		args = append(args, ticket)
	}
	if passenger != "" {
		where += " AND p.name LIKE ?"
		args = append(args, escapeLike(passenger)+"%")
	}
	if basis == "processing" {
		location, err := businessLocation()
		if err != nil {
			return nil, 0, err
		}
		start, end, err := importedBounds(from, to, location)
		if err != nil {
			return nil, 0, err
		}
		where += " AND r.imported_at>=? AND r.imported_at<?"
		args = append(args, start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
	} else {
		where += " AND " + issueDateSQL + ">=? AND " + issueDateSQL + "<=?"
		args = append(args, from, to)
	}
	var total int
	if err := repo.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+captureJoin+" WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := repo.DB.QueryContext(ctx, "SELECT "+captureColumns+captureJoin+" WHERE "+where+" ORDER BY r.imported_at DESC,r.id DESC,p.id LIMIT ? OFFSET ?", append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []CaptureRow{}
	for rows.Next() {
		row, err := scanCapture(rows)
		if err != nil {
			return nil, 0, err
		}
		// A web reference has no verified mapping in the MIR layout. Keep it blank.
		row.ClientReference = strings.TrimSpace(row.ClientReference)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if err := repo.enrichCapture(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type captureScanner interface{ Scan(...any) error }

func scanCapture(scanner captureScanner) (CaptureRow, error) {
	var row CaptureRow
	err := scanner.Scan(&row.RecordID, &row.PassengerID, &row.TicketNumber, &row.PNR, &row.GDS, &row.Staff, &row.ClientReference, &row.InvoiceNumber, &row.Branch, &row.PassengerName, &row.IssueDate, &row.IssueDateSource, &row.Sector, &row.Fare, &row.Currency, &row.ProcessingDate, &row.ImportedAt, &row.importStatus, &row.validationJSON, &row.airlineSource)
	return row, err
}

func (h *StageHandler) capture(w http.ResponseWriter, r *http.Request) {
	f, basis, err := captureParams(r.URL.Query())
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	location, err := businessLocation()
	if err != nil {
		failure(w, 503, "The business timezone is invalid. Ask your administrator to check BUSINESS_TIMEZONE.")
		return
	}
	rows, total, err := repo.Capture(ctx, f, basis)
	if err != nil {
		failure(w, 503, "gds_capture_unavailable")
		return
	}
	actor, _ := auth.PrincipalFromContext(r.Context())
	canView, err := repo.canViewSales(ctx, actor.ID)
	if err != nil {
		failure(w, 503, "Sales permissions could not be checked. Try again.")
		return
	}
	for i := range rows {
		rows[i].Progress.CanViewSale = canView
	}
	respond(w, 200, map[string]any{"records": rows, "total": total, "page": f.Page, "page_size": f.PageSize, "date_basis": basis, "timezone": location.String(), "searched_at": time.Now().UTC().Format(time.RFC3339)})
}
