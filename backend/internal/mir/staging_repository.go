package mir

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/mir/decoder"
)

type Repository struct{ DB *sql.DB }

func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func nullableDate(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (repo Repository) StartBatch(ctx context.Context, actor []byte) (int64, error) {
	res, e := repo.DB.ExecContext(ctx, `INSERT INTO mir_import_batch(errors,created_by) VALUES(JSON_ARRAY(),?)`, actor)
	if e != nil {
		return 0, e
	}
	return res.LastInsertId()
}
func (repo Repository) FinishBatch(ctx context.Context, b Batch) error {
	_, e := repo.DB.ExecContext(ctx, `UPDATE mir_import_batch SET status=?,imported_count=?,duplicate_count=?,failed_count=?,errors=?,completed_at=UTC_TIMESTAMP(6) WHERE id=?`, b.Status, b.Imported, b.Duplicates, b.Failed, jsonText(b.Errors), b.ID)
	return e
}

func (repo Repository) Source(ctx context.Context, batch int64, name string, data []byte) (int64, bool, error) {
	hash := sha256.Sum256(data)
	res, e := repo.DB.ExecContext(ctx, `INSERT INTO mir_source_file(batch_id,filename,sha256,original_bytes,byte_size) VALUES(?,?,?,?,?)`, batch, name, hex.EncodeToString(hash[:]), data, len(data))
	if e != nil {
		var me *mysql.MySQLError
		if errors.As(e, &me) && me.Number == 1062 {
			var id int64
			e = repo.DB.QueryRowContext(ctx, `SELECT id FROM mir_source_file WHERE sha256=?`, hex.EncodeToString(hash[:])).Scan(&id)
			return id, true, e
		}
		return 0, false, e
	}
	id, e := res.LastInsertId()
	return id, false, e
}

type persistedItem struct {
	id, section int64
	source      string
	number      int
	values      map[string]string
}

func (repo Repository) Stage(ctx context.Context, source int64, number int, r *decoder.Result, reprocess int64) (int64, error) {
	tx, e := repo.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	id, e := stageRecord(ctx, tx, source, number, r, reprocess)
	if e != nil {
		return 0, e
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return id, nil
}
func stageRecord(ctx context.Context, tx *sql.Tx, source int64, number int, r *decoder.Result, reprocess int64) (int64, error) {
	header := r.Values("HEADER", 1)
	issues := append([]decoder.Issue{}, r.Issues...)
	mirDate, e := decoder.Date(header["T50DTE"])
	if e != nil {
		issues = append(issues, decoder.Issue{Severity: "error", Code: "MIR_DATE", Message: "T50DTE is absent or invalid; no fallback date used"})
	}
	travelDate := ""
	if header["T50DFT"] != "" {
		travelDate, e = decoder.Date(header["T50DFT"])
		if e != nil {
			issues = append(issues, decoder.Issue{Severity: "warning", Code: "TRAVEL_DATE", Message: "T50DFT cannot be normalized; original header value retained"})
		}
	}
	status := "staged"
	for _, issue := range issues {
		if status != "failed" {
			status = "review"
		}
		if issue.Severity == "error" {
			status = "failed"
		}
	}
	passengers, tickets := []string{}, []string{}
	currency, amount := "", ""
	fareCount := 0
	for _, it := range r.Items {
		v := r.Values(it.Section, it.Number)
		if it.Section == "A02" {
			name := v["A02EXN"]
			if name == "" {
				name = v["A02NME"]
			}
			if name != "" {
				passengers = append(passengers, name)
			}
			ticket := v["A02TKT"]
			if len(header["T50ISA"]) == 3 && len(ticket) == 10 {
				ticket = header["T50ISA"] + ticket
			}
			if ticket != "" {
				tickets = append(tickets, ticket)
			}
		}
		if it.Section == "A07" {
			fareCount++
			currency = v["A07CRT"]
			amount = v["A07TTA"]
		}
	}
	if fareCount != 1 {
		currency = ""
		amount = ""
	}
	if len(tickets) == 0 {
		for _, it := range r.Items {
			v := r.Values(it.Section, it.Number)
			var doc, name string
			switch it.Section {
			case "A29":
				doc = v["A29EMD"]
				name = v["A29NAM"]
			case "A23":
				doc = v["A23TKT"]
				name = v["A23NME"]
			case "A32":
				doc = v["A32SFN"]
				name = v["A32NME"]
			}
			if doc != "" {
				tickets = append(tickets, doc)
			}
			if len(passengers) == 0 && name != "" {
				passengers = append(passengers, name)
			}
		}
	}
	if len(passengers) > 1 {
		passengers = []string{fmt.Sprintf("%s (+%d)", passengers[0], len(passengers)-1)}
	}
	if len(tickets) > 1 {
		tickets = []string{fmt.Sprintf("%s (+%d)", tickets[0], len(tickets)-1)}
	}
	id := reprocess
	args := []any{nullableDate(mirDate), nullableDate(travelDate), header["T50RCL"], header["T50IN12"], strings.Join(passengers, ""), strings.Join(tickets, ""), currency, amount, status, decoder.Version, r.Raw, jsonText(header), jsonText(issues)}
	if id == 0 {
		res, e := tx.ExecContext(ctx, `INSERT INTO mir_record(mir_date,first_travel_date,locator,mir_type,passenger,ticket_number,currency,fare_amount,status,parser_version,raw_record,decoded_header,validation_issues,source_file_id,record_number) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, source, number)...)
		if e != nil {
			return 0, e
		}
		id, e = res.LastInsertId()
		if e != nil {
			return 0, e
		}
	} else {
		var existing int64
		if e := tx.QueryRowContext(ctx, `SELECT id FROM mir_record WHERE id=? FOR UPDATE`, id).Scan(&existing); e != nil {
			return 0, e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM mir_section WHERE record_id=?`, id); e != nil {
			return 0, e
		}
		if _, e := tx.ExecContext(ctx, `UPDATE mir_record SET mir_date=?,first_travel_date=?,locator=?,mir_type=?,passenger=?,ticket_number=?,currency=?,fare_amount=?,status=?,parser_version=?,raw_record=?,decoded_header=?,validation_issues=?,reprocessed_at=UTC_TIMESTAMP(6) WHERE id=?`, append(args, id)...); e != nil {
			return 0, e
		}
	}
	items := []persistedItem{}
	// Each physical repeated item is represented as a section occurrence. Blank
	// section terminators are preserved in raw_record; each raw item includes CR.
	addItem := func(section string, n int, raw []byte) (persistedItem, error) {
		c := decoder.SectionCoverage(section)
		if section == "HEADER" {
			c = decoder.Coverage{Section: section, Status: "partial", Reason: "Reviewed five-line MIR92 header"}
		}
		if c.Status != "decoded" && section != "HEADER" && status == "staged" {
			status = "review"
		}
		res, e := tx.ExecContext(ctx, `INSERT INTO mir_section(record_id,identifier,occurrence,decoder_status,decoder_reason,raw_data) VALUES(?,?,?,?,?,?)`, id, section, n, c.Status, c.Reason, raw)
		if e != nil {
			return persistedItem{}, e
		}
		sid, e := res.LastInsertId()
		if e != nil {
			return persistedItem{}, e
		}
		res, e = tx.ExecContext(ctx, `INSERT INTO mir_section_item(section_id,record_id,item_number,raw_data) VALUES(?,?,?,?)`, sid, id, n, raw)
		if e != nil {
			return persistedItem{}, e
		}
		iid, e := res.LastInsertId()
		if e != nil {
			return persistedItem{}, e
		}
		for _, f := range r.Fields {
			if f.Section != section || f.Item != n {
				continue
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO mir_decoded_field(item_id,label,field_name,data_type,raw_value,decoded_value,byte_start,byte_end,status,guide_reference,note) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, iid, f.Label, f.Name, f.Type, r.Raw[f.Start:f.End], jsonText(f.Value), f.Start, f.End, f.Status, f.Reference, f.Note); e != nil {
				return persistedItem{}, e
			}
		}
		return persistedItem{iid, sid, section, n, r.Values(section, n)}, nil
	}
	headerEnd := len(r.Raw)
	if len(r.Items) > 0 {
		headerEnd = r.Items[0].Start
	}
	if _, e := addItem("HEADER", 1, r.Raw[:headerEnd]); e != nil {
		return 0, e
	}
	for _, it := range r.Items {
		p, e := addItem(it.Section, it.Number, r.ItemRaw(it))
		if e != nil {
			return 0, e
		}
		items = append(items, p)
		for _, m := range typedMappings {
			match := false
			for _, s := range m.Sections {
				if s == it.Section {
					match = true
				}
			}
			if !match {
				continue
			}
			columns := []string{"record_id", "section_id", "source_item_id", "decoded_data", "mapping_status"}
			params := []any{id, p.section, p.id, jsonText(p.values), decoder.SectionCoverage(it.Section).Status}
			keys := []string{}
			for col := range m.Columns {
				keys = append(keys, col)
			}
			sort.Strings(keys)
			for _, col := range keys {
				columns = append(columns, col)
				label := m.Columns[col]
				if it.Section == "A05" {
					label = strings.Replace(label, "A04", "A05", 1)
					if col == "departure_date" {
						label = "A05DTE"
					}
				}
				if it.Section == "A31" && col == "segment_ref" {
					label = "A31NUM"
				}
				if it.Section == "A28" {
					label = strings.Replace(label, "A07", "A28", 1)
				}
				if it.Section == "A24" {
					label = strings.Replace(label, "A09", "A24", 1)
				}
				if col == "remark_text" {
					switch it.Section {
					case "A00":
						label = "A00CUS"
					case "A01":
						label = "A01CPI"
					case "A15":
						label = "A15RMK"
					}
				}
				value := p.values[label]
				if m.Table == "passenger" && col == "name" && p.values["A02EXN"] != "" {
					value = p.values["A02EXN"]
				}
				if m.Table == "passenger" && col == "ticket_number" && len(value) == 10 && len(header["T50ISA"]) == 3 {
					value = header["T50ISA"] + value
				}
				params = append(params, value)
			}
			placeholders := strings.TrimRight(strings.Repeat("?,", len(params)), ",")
			if _, e := tx.ExecContext(ctx, "INSERT INTO mir_"+m.Table+"("+strings.Join(columns, ",")+") VALUES("+placeholders+")", params...); e != nil {
				return 0, e
			}
		}
		if it.Section == "A07" {
			for label, value := range p.values {
				codeLabel := ""
				if strings.HasPrefix(label, "A07TT") && len(label) == 6 && label[5] >= '1' && label[5] <= '5' {
					codeLabel = strings.Replace(label, "TT", "TC", 1)
				} else if strings.HasPrefix(label, "A07IT") || strings.HasPrefix(label, "A07TP") || strings.HasPrefix(label, "A07TN") || strings.HasPrefix(label, "A07ET") {
					if !strings.HasSuffix(label, "C") {
						codeLabel = label + "C"
					}
				}
				if codeLabel == "" || value == "" {
					continue
				}
				if _, e := tx.ExecContext(ctx, `INSERT INTO mir_fare_tax(record_id,section_id,source_item_id,source_label,tax_code,amount,currency) VALUES(?,?,?,?,?,?,?)`, id, p.section, p.id, label, p.values[codeLabel], value, p.values["A07CUR"]); e != nil {
					return 0, e
				}
			}
		}
	}
	// Only explicit guide identifiers join items. Multiple competing targets are
	// deliberately unresolved; physical row order is never an association.
	link := func(from persistedItem, label, toSection, target, kind string) error {
		ref := from.values[label]
		if ref == "" {
			return nil
		}
		var match *persistedItem
		for i := range items {
			p := &items[i]
			if p.source == toSection && p.values[target] == ref {
				if match != nil {
					return nil
				}
				match = p
			}
		}
		if match == nil {
			return nil
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO mir_item_relationship(record_id,from_item_id,from_section_id,to_item_id,to_section_id,relationship_type,source_label) VALUES(?,?,?,?,?,?,?)`, id, from.id, from.section, match.id, match.section, kind, label)
		return e
	}
	for _, it := range items {
		switch it.source {
		case "A02":
			e = link(it, "A02FIN", "A07", "A07FSI", "passenger_fare")
			if e == nil {
				e = link(it, "A02EIN", "A10", "A10EXI", "passenger_exchange")
			}
		case "A08":
			e = link(it, "A08FSI", "A07", "A07FSI", "basis_fare")
			if e == nil {
				e = link(it, "A08ITN", "A04", "A04ITN", "basis_segment")
			}
		case "A09":
			e = link(it, "A09FSI", "A07", "A07FSI", "construction_fare")
		case "A15":
			e = link(it, "A15SEG", "A04", "A04ITN", "remark_segment")
		case "A20":
			e = link(it, "A20SEG", "A04", "A04ITN", "ssr_segment")
		case "A30":
			e = link(it, "A30EMD", "A29", "A29EMD", "coupon_emd")
		}
		if e != nil {
			return 0, e
		}
	}
	// Materialize passenger -> segment through its explicitly associated fare and
	// fare basis; do not guess passenger number from item ordinal.
	_, e = tx.ExecContext(ctx, `INSERT INTO mir_item_relationship(record_id,from_item_id,from_section_id,to_item_id,to_section_id,relationship_type,source_label)
 SELECT DISTINCT pf.record_id,pf.from_item_id,pf.from_section_id,bs.to_item_id,bs.to_section_id,'passenger_segment','A02FIN/A08FSI/A08ITN'
 FROM mir_item_relationship pf JOIN mir_item_relationship bf ON bf.to_item_id=pf.to_item_id AND bf.relationship_type='basis_fare'
 JOIN mir_item_relationship bs ON bs.from_item_id=bf.from_item_id AND bs.relationship_type='basis_segment'
 WHERE pf.record_id=? AND pf.relationship_type='passenger_fare'`, id)
	if e != nil {
		return 0, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE mir_record SET status=? WHERE id=?`, status, id)
	return id, e
}

type RecordFilter struct {
	From, To, Status, Locator, Ticket, Passenger, Filename, Search string
	Page, PageSize                                                 int
}

func (f RecordFilter) where() (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	add := func(s string, v any) { clauses = append(clauses, s); args = append(args, v) }
	if f.From != "" {
		add("r.mir_date>=?", f.From)
	}
	if f.To != "" {
		add("r.mir_date<=?", f.To)
	}
	if f.Status != "" {
		add("r.status=?", f.Status)
	}
	if f.Locator != "" {
		add("r.locator=?", f.Locator)
	}
	if f.Ticket != "" {
		clauses = append(clauses, "(EXISTS(SELECT 1 FROM mir_passenger p WHERE p.record_id=r.id AND p.ticket_number=?) OR EXISTS(SELECT 1 FROM mir_refund rf WHERE rf.record_id=r.id AND rf.ticket_number=?) OR EXISTS(SELECT 1 FROM mir_emd_coupon ec WHERE ec.record_id=r.id AND ec.ticket_number=?))")
		args = append(args, f.Ticket, f.Ticket, f.Ticket)
	}
	if f.Passenger != "" {
		add("EXISTS(SELECT 1 FROM mir_passenger p WHERE p.record_id=r.id AND p.name LIKE ?)", escapeLike(f.Passenger)+"%")
	}
	if f.Filename != "" {
		add("s.filename LIKE ?", escapeLike(f.Filename)+"%")
	}
	if f.Search != "" {
		v := escapeLike(f.Search) + "%"
		clauses = append(clauses, "(r.locator LIKE ? OR s.filename LIKE ? OR r.passenger LIKE ? OR r.ticket_number LIKE ? OR EXISTS(SELECT 1 FROM mir_passenger p WHERE p.record_id=r.id AND (p.name LIKE ? OR p.ticket_number LIKE ?)))")
		args = append(args, v, v, v, v, v, v)
	}
	return strings.Join(clauses, " AND "), args
}
func escapeLike(v string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(v)
}

const rowColumns = `r.id,r.source_file_id,s.batch_id,COALESCE(DATE_FORMAT(r.mir_date,'%Y-%m-%d'),''),COALESCE(DATE_FORMAT(r.first_travel_date,'%Y-%m-%d'),''),r.locator,r.mir_type,r.passenger,r.ticket_number,r.currency,r.fare_amount,r.status,s.filename,r.parser_version`

type scanner interface{ Scan(...any) error }

func scanStage(s scanner) (StageRow, error) {
	var row StageRow
	e := s.Scan(&row.ID, &row.SourceFileID, &row.BatchID, &row.MIRDate, &row.FirstTravelDate, &row.Locator, &row.MIRType, &row.Passenger, &row.TicketNumber, &row.Currency, &row.FareAmount, &row.Status, &row.Filename, &row.ParserVersion)
	return row, e
}
func (repo Repository) List(ctx context.Context, f RecordFilter) ([]StageRow, int, error) {
	where, args := f.where()
	join := " FROM mir_record r JOIN mir_source_file s ON s.id=r.source_file_id WHERE " + where
	var total int
	if e := repo.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+join, args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	rows, e := repo.DB.QueryContext(ctx, "SELECT "+rowColumns+join+" ORDER BY r.mir_date DESC,r.id DESC LIMIT ? OFFSET ?", append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []StageRow{}
	for rows.Next() {
		r, e := scanStage(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func maps(ctx context.Context, db queryer, query string, args ...any) ([]map[string]any, error) {
	rows, e := db.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	cols, e := rows.Columns()
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for rows.Next() {
		v := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range v {
			ptr[i] = &v[i]
		}
		if e := rows.Scan(ptr...); e != nil {
			return nil, e
		}
		row := map[string]any{}
		for i, c := range cols {
			if b, ok := v[i].([]byte); ok {
				row[c] = string(b)
			} else {
				row[c] = v[i]
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
func (repo Repository) Detail(ctx context.Context, id int64) (StageDetail, error) {
	d := StageDetail{Typed: map[string][]map[string]any{}, Relationships: []map[string]any{}, Sections: []map[string]any{}, Coverage: decoder.Registry()}
	tx, e := repo.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	d.StageRow, e = scanStage(tx.QueryRowContext(ctx, "SELECT "+rowColumns+` FROM mir_record r JOIN mir_source_file s ON s.id=r.source_file_id WHERE r.id=?`, id))
	if e != nil {
		return d, e
	}
	var header, issues, raw []byte
	if e = tx.QueryRowContext(ctx, `SELECT decoded_header,validation_issues,raw_record FROM mir_record WHERE id=?`, id).Scan(&header, &issues, &raw); e != nil {
		return d, e
	}
	if e = json.Unmarshal(header, &d.Header); e != nil {
		return d, e
	}
	if e = json.Unmarshal(issues, &d.Issues); e != nil {
		return d, e
	}
	d.Raw = latinText(raw)
	d.RawHex = hex.EncodeToString(raw)
	for _, m := range typedMappings {
		rows, e := maps(ctx, tx, "SELECT * FROM mir_"+m.Table+" WHERE record_id=? ORDER BY id", id)
		if e != nil {
			return d, e
		}
		for _, row := range rows {
			if value, ok := row["decoded_data"].(string); ok {
				var data any
				if json.Unmarshal([]byte(value), &data) == nil {
					row["decoded_data"] = data
				}
			}
		}
		d.Typed[m.Table] = rows
	}
	d.Typed["fare_tax"], e = maps(ctx, tx, `SELECT * FROM mir_fare_tax WHERE record_id=? ORDER BY id`, id)
	if e != nil {
		return d, e
	}
	d.Relationships, e = maps(ctx, tx, `SELECT * FROM mir_item_relationship WHERE record_id=? ORDER BY id`, id)
	if e != nil {
		return d, e
	}
	d.Sections, e = maps(ctx, tx, `SELECT id,identifier,occurrence,decoder_status,decoder_reason,HEX(raw_data) AS raw_hex FROM mir_section WHERE record_id=? ORDER BY id`, id)
	if e != nil {
		return d, e
	}
	for _, s := range d.Sections {
		items, e := maps(ctx, tx, `SELECT id,item_number,HEX(raw_data) AS raw_hex FROM mir_section_item WHERE section_id=? ORDER BY id`, s["id"])
		if e != nil {
			return d, e
		}
		s["items"] = items
		for _, it := range items {
			fields, e := maps(ctx, tx, `SELECT label,field_name,data_type,HEX(raw_value) AS raw_hex,decoded_value,byte_start,byte_end,status,guide_reference,note FROM mir_decoded_field WHERE item_id=? ORDER BY byte_start,id`, it["id"])
			if e != nil {
				return d, e
			}
			it["fields"] = fields
		}
	}
	return d, tx.Commit()
}
func latinText(raw []byte) string {
	r := make([]rune, len(raw))
	for i, b := range raw {
		r[i] = rune(b)
	}
	return string(r)
}
func (repo Repository) Batches(ctx context.Context, id int64, page int) ([]Batch, error) {
	query := `SELECT id,status,imported_count,duplicate_count,failed_count,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),errors FROM mir_import_batch`
	args := []any{}
	if id > 0 {
		query += " WHERE id=?"
		args = append(args, id)
	}
	query += " ORDER BY id DESC LIMIT 50 OFFSET ?"
	args = append(args, (page-1)*50)
	rows, e := repo.DB.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Batch{}
	for rows.Next() {
		var b Batch
		var raw []byte
		if e := rows.Scan(&b.ID, &b.Status, &b.Imported, &b.Duplicates, &b.Failed, &b.CreatedAt, &raw); e != nil {
			return nil, e
		}
		if e := json.Unmarshal(raw, &b.Errors); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func parseID(v string) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("invalid id")
	}
	return n, nil
}
