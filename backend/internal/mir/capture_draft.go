package mir

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"voltafinbook/backend/internal/mir/decoder"
)

type DraftField struct {
	Value    string   `json:"value"`
	Lines    []int    `json:"lines"`
	FieldIDs []string `json:"fieldIds"`
}
type DraftLine struct {
	Number        int    `json:"number"`
	Text          string `json:"text"`
	RawHex        string `json:"rawHex"`
	TerminatorHex string `json:"terminatorHex"`
}
type CaptureDraft struct {
	ID         string                `json:"id"`
	FileName   string                `json:"fileName"`
	SHA256     string                `json:"sha256"`
	RecordDate string                `json:"recordDate"`
	Status     string                `json:"status"`
	Fields     map[string]DraftField `json:"fields"`
	RawLines   []DraftLine           `json:"rawLines"`
	Issues     []string              `json:"issues"`
}
type draftItem struct {
	section string
	number  int
}

var errFailedCapture = errors.New("failed MIR record requires review before proceeding")
var errSaleExists = errors.New("A sale already exists for this imported ticket. Open the linked sale instead of preparing another draft.")
var draftDecimal = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
var draftDigits = regexp.MustCompile(`^\d+$`)

// CaptureDraft resolves the persisted passenger identity and explicit fare and
// journey relationships in one read-only snapshot. It creates no sales record.
func (repo Repository) CaptureDraft(ctx context.Context, recordID, passengerID int64) (CaptureDraft, error) {
	return repo.captureDraft(ctx, recordID, passengerID, false)
}
func (repo Repository) captureDraft(ctx context.Context, recordID, passengerID int64, reviewOnly bool) (CaptureDraft, error) {
	tx, err := repo.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return CaptureDraft{}, err
	}
	defer tx.Rollback()
	var raw, issues []byte
	var filename, status string
	var date sql.NullTime
	var sourceItem int64
	var passengerItem int
	var ticketNumber string
	err = tx.QueryRowContext(ctx, `SELECT r.raw_record,s.filename,r.mir_date,r.status,r.validation_issues,p.source_item_id,item.item_number
 ,p.ticket_number
 FROM mir_passenger p JOIN mir_record r ON r.id=p.record_id JOIN mir_source_file s ON s.id=r.source_file_id
 JOIN mir_section_item item ON item.id=p.source_item_id
 WHERE p.id=? AND r.id=?`, passengerID, recordID).Scan(&raw, &filename, &date, &status, &issues, &sourceItem, &passengerItem, &ticketNumber)
	if err != nil {
		return CaptureDraft{}, err
	}
	if !reviewOnly {
		sales, err := linkedSales(ctx, tx, []string{ticketNumber})
		if err != nil {
			return CaptureDraft{}, err
		}
		if _, exists := sales[ticketKey(ticketNumber)]; exists {
			return CaptureDraft{}, errSaleExists
		}
	}
	if status == "failed" {
		return CaptureDraft{}, errFailedCapture
	}
	linked := func(kind string) ([]draftItem, error) {
		rows, err := tx.QueryContext(ctx, `SELECT section.identifier,item.item_number FROM mir_item_relationship link
 JOIN mir_section_item item ON item.id=link.to_item_id JOIN mir_section section ON section.id=item.section_id
 WHERE link.record_id=? AND link.from_item_id=? AND link.relationship_type=? ORDER BY item.item_number,item.id`, recordID, sourceItem, kind)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []draftItem{}
		for rows.Next() {
			var item draftItem
			if err := rows.Scan(&item.section, &item.number); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, rows.Err()
	}
	fares, err := linked("passenger_fare")
	if err != nil {
		return CaptureDraft{}, err
	}
	segments, err := linked("passenger_segment")
	if err != nil {
		return CaptureDraft{}, err
	}
	parsed, err := decoder.ParseStream(raw, filename)
	if err != nil || len(parsed) != 1 {
		return CaptureDraft{}, errors.New("selected MIR source cannot be resolved safely")
	}
	draft := buildCaptureDraft(parsed[0], passengerItem, fares, segments)
	draft.ID = fmt.Sprintf("mir:%d:passenger:%d", recordID, passengerID)
	if date.Valid {
		draft.RecordDate = date.Time.Format("2006-01-02")
	}
	var storedIssues []decoder.Issue
	if err := json.Unmarshal(issues, &storedIssues); err != nil {
		return CaptureDraft{}, err
	}
	seen := map[string]bool{}
	for _, issue := range draft.Issues {
		seen[issue] = true
	}
	for _, issue := range storedIssues {
		message := issue.Code + ": " + issue.Message
		if !seen[message] {
			draft.Issues = append(draft.Issues, message)
			seen[message] = true
		}
	}
	if status == "review" || len(draft.Issues) > 0 {
		draft.Status = "Validation required"
	}
	if err := tx.Commit(); err != nil {
		return CaptureDraft{}, err
	}
	return draft, nil
}

func buildCaptureDraft(r *decoder.Result, passenger int, fares, segments []draftItem) CaptureDraft {
	d := CaptureDraft{FileName: r.Filename, SHA256: r.SHA256, Status: "Extracted", Fields: map[string]DraftField{}, RawLines: []DraftLine{}, Issues: []string{}}
	for _, line := range r.Lines {
		d.RawLines = append(d.RawLines, DraftLine{Number: line.Number, Text: latinText(line.Data), RawHex: hex.EncodeToString(line.Data), TerminatorHex: hex.EncodeToString(line.Ending)})
	}
	find := func(label string, item int) *decoder.Field {
		var result *decoder.Field
		for i := range r.Fields {
			f := &r.Fields[i]
			if f.Label == label && f.Item == item && f.Status == "parsed" && strings.TrimSpace(f.Raw) != "" {
				if result != nil {
					return nil
				}
				result = f
			}
		}
		return result
	}
	assign := func(key, label string, item int, money bool) {
		f := find(label, item)
		if f == nil {
			return
		}
		value := strings.TrimSpace(f.Raw)
		if money && !draftDecimal.MatchString(value) {
			d.Issues = append(d.Issues, label+": unsupported amount retained in source.")
			return
		}
		d.Fields[key] = DraftField{Value: value, Lines: []int{f.Line}, FieldIDs: []string{f.ID}}
	}
	assign("ticketNumber", "A02TKT", passenger, false)
	carrier, serial := find("T50ISA", 1), find("A02TKT", passenger)
	if carrier != nil && serial != nil {
		prefix, number := strings.TrimSpace(carrier.Raw), strings.TrimSpace(serial.Raw)
		if len(prefix) == 3 && len(number) == 10 && draftDigits.MatchString(prefix) && draftDigits.MatchString(number) {
			lines := []int{carrier.Line}
			if serial.Line != carrier.Line {
				lines = append(lines, serial.Line)
			}
			sort.Ints(lines)
			d.Fields["ticketNumber"] = DraftField{Value: prefix + number, Lines: lines, FieldIDs: []string{carrier.ID, serial.ID}}
		}
	}
	assign("passengerName", "A02EXN", passenger, false)
	if _, ok := d.Fields["passengerName"]; !ok {
		assign("passengerName", "A02NME", passenger, false)
	}
	assign("pnr", "T50RCL", 1, false)
	assign("airline", "T50ISC", 1, false)
	if _, ok := d.Fields["airline"]; !ok {
		assign("airline", "T50ISN", 1, false)
	}
	assign("ticketDate", "A02DTE", passenger, false)
	if len(fares) == 1 && fares[0].section == "A07" {
		item := fares[0].number
		assign("basicFare", "A07TBF", item, true)
		assign("basicCurrency", "A07CRB", item, false)
		assign("mirTotalFare", "A07TTA", item, true)
		assign("fareCurrency", "A07CRT", item, false)
	}
	for _, key := range []string{"sector", "travelDate"} {
		values := []string{}
		lines := map[int]bool{}
		ids := []string{}
		for _, segment := range segments {
			if segment.section != "A04" {
				continue
			}
			labels := []string{"A04DTL"}
			if key == "sector" {
				labels = []string{"A04OCC", "A04DCC"}
			}
			fields := []*decoder.Field{}
			complete := true
			for _, label := range labels {
				f := find(label, segment.number)
				if f == nil {
					complete = false
					break
				}
				fields = append(fields, f)
			}
			if !complete {
				continue
			}
			parts := []string{}
			for _, f := range fields {
				parts = append(parts, strings.TrimSpace(f.Raw))
				lines[f.Line] = true
				ids = append(ids, f.ID)
			}
			values = append(values, strings.Join(parts, " → "))
		}
		if len(values) > 0 {
			ordered := []int{}
			for line := range lines {
				ordered = append(ordered, line)
			}
			sort.Ints(ordered)
			d.Fields[key] = DraftField{Value: strings.Join(values, " / "), Lines: ordered, FieldIDs: ids}
		}
	}
	if _, ok := d.Fields["ticketNumber"]; !ok {
		d.Issues = append(d.Issues, "Ticket number is missing or ambiguous. Complete it before saving.")
	}
	if len(d.Issues) > 0 {
		d.Status = "Validation required"
	}
	return d
}

func (h *StageHandler) captureDraft(w http.ResponseWriter, r *http.Request) {
	passengerID, err := parseID(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "invalid_passenger_id")
		return
	}
	recordID, err := parseID(r.URL.Query().Get("record_id"))
	if err != nil || len(r.URL.Query()["record_id"]) != 1 {
		failure(w, 400, "invalid_record_id")
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	draft, err := repo.CaptureDraft(ctx, recordID, passengerID)
	if errors.Is(err, errSaleExists) {
		failure(w, 409, err.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		failure(w, 404, "Selected ticket changed or is no longer available. Refresh GDS Capture and select it again.")
		return
	}
	if errors.Is(err, errFailedCapture) {
		failure(w, 409, "This ticket failed extraction validation. Review it in MIR Import Report before preparing a sale.")
		return
	}
	if err != nil {
		failure(w, 503, "Selected ticket could not be loaded. Try again.")
		return
	}
	respond(w, 200, draft)
}
