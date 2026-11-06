package mir

import "voltafinbook/backend/internal/mir/decoder"

type typedMapping struct {
	Table    string
	Sections []string
	Columns  map[string]string
}

var typedMappings = []typedMapping{
	{"passenger", []string{"A02"}, map[string]string{"name": "A02NME", "ticket_number": "A02TKT", "passenger_ref": "", "fare_ref": "A02FIN"}},
	{"frequent_flyer", []string{"A03"}, map[string]string{"name": "A03FFP", "carrier": "A03FCC", "membership_number": "A03FFN"}},
	{"segment", []string{"A04", "A05", "A31"}, map[string]string{"segment_ref": "A04ITN", "carrier": "A04CDE", "origin": "A04OCC", "destination": "A04DCC", "departure_date": "A04DTL"}},
	{"fare", []string{"A07", "A28"}, map[string]string{"fare_ref": "A07FSI", "currency": "A07CRT", "base_amount": "A07TBF", "total_amount": "A07TTA"}},
	{"fare_basis", []string{"A08"}, map[string]string{"fare_ref": "A08FSI", "segment_ref": "A08ITN", "basis_code": "A08FBC"}},
	{"fare_construction", []string{"A09", "A24"}, map[string]string{"fare_ref": "A09FSI", "construction_text": "A09L51"}},
	{"payment", []string{"A11"}, map[string]string{"payment_type": "A11TYP", "amount": "A11AMT", "passenger_ref": "A11PGR"}},
	{"ticket_exchange", []string{"A10"}, map[string]string{"exchange_ref": "A10EXI"}},
	{"refund", []string{"A23"}, map[string]string{"ticket_number": "A23TKT", "name": "A23NME", "issue_date": "A23DOI"}},
	{"emd", []string{"A29"}, map[string]string{"document_number": "A29EMD", "passenger_ref": "A29PAX", "segment_ref": "A29SEG", "currency": "A29CTA", "total_amount": "A29TLA"}},
	{"emd_coupon", []string{"A30"}, map[string]string{"document_number": "A30EMD", "coupon_number": "A30CPN", "passenger_ref": "A30PAX", "segment_ref": "A30SEG", "ticket_number": "A30ATK"}},
	{"auxiliary_segment", []string{"A16", "A17"}, map[string]string{}},
	{"phone", []string{"A12"}, map[string]string{"city": "A12CTY", "phone": "A12PHN"}},
	{"address", []string{"A13"}, map[string]string{"address_text": "A13DTA"}},
	{"remark", []string{"A00", "A01", "A14", "A15"}, map[string]string{"remark_text": "A14RMK", "segment_ref": "A15SEG"}},
	{"ssr_osi", []string{"A20"}, map[string]string{"segment_ref": "A20SEG", "passenger_ref": "A20PAX", "service_type": "A20CDE", "service_status": "A20STS", "carrier": "A20AIR", "service_text": "A20TXT"}},
	{"carrier_fee", []string{"A27"}, map[string]string{}},
	{"misc_document", []string{"A19", "A21", "A22", "A26", "A32", "A06"}, map[string]string{}},
}

type StageRow struct {
	ID              int64  `json:"id"`
	SourceFileID    int64  `json:"source_file_id"`
	BatchID         int64  `json:"batch_id"`
	MIRDate         string `json:"mir_date"`
	FirstTravelDate string `json:"first_travel_date"`
	Locator         string `json:"locator"`
	MIRType         string `json:"mir_type"`
	Passenger       string `json:"passenger"`
	TicketNumber    string `json:"ticket_number"`
	Currency        string `json:"currency"`
	FareAmount      string `json:"fare_amount"`
	Status          string `json:"status"`
	Filename        string `json:"filename"`
	ParserVersion   string `json:"parser_version"`
}
type StageDetail struct {
	StageRow
	Header        map[string]string           `json:"header"`
	Typed         map[string][]map[string]any `json:"typed"`
	Relationships []map[string]any            `json:"relationships"`
	Sections      []map[string]any            `json:"sections"`
	Raw           string                      `json:"raw"`
	RawHex        string                      `json:"raw_hex"`
	Issues        []decoder.Issue             `json:"issues"`
	Coverage      []decoder.Coverage          `json:"coverage"`
}
type ImportError struct {
	Filename string `json:"filename"`
	Record   int    `json:"record"`
	Message  string `json:"message"`
}
type Batch struct {
	ID         int64         `json:"id"`
	Status     string        `json:"status"`
	Imported   int           `json:"imported_count"`
	Duplicates int           `json:"duplicate_count"`
	Failed     int           `json:"failed_count"`
	CreatedAt  string        `json:"created_at"`
	Errors     []ImportError `json:"errors"`
}
