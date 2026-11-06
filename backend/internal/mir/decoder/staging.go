package decoder

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const Version = "finbook-mir-1.0.0"

type Coverage struct{ Section, Status, Reason string }

// Registry enumerates every guide section, including discontinued A18. Partial
// means that raw data must remain available; it is never a claim of full coverage.
func Registry() []Coverage {
	out := []Coverage{}
	for n := 0; n <= 32; n++ {
		if n == 25 {
			continue
		}
		s := fmt.Sprintf("A%02d", n)
		reason := "Layout not yet transcribed and verified from guide; raw items retained"
		status := "unsupported"
		switch s {
		case "A00":
			status = "partial"
			reason = "Customer remark and email; client-file continuation remains unresolved"
		case "A01", "A03", "A14", "A15":
			status = "partial"
			reason = "Base documented item decoded; additional continuation bytes retained"
		case "A02":
			status = "partial"
			reason = "Base and optional tags; TD spelling, TLD width and FFN displacement conflict in guide"
		case "A04":
			status = "partial"
			reason = "Normal air layout and optional tags; surface, taxi and rail alternatives unresolved"
		case "A07", "A08", "A09":
			status = "partial"
			reason = "Reviewed fare, tax and basis layouts; VAT and additional continuation layouts retained"
		case "A11":
			status = "partial"
			reason = "Host-specific approval widths and optional tags; conflicting early tag boundary marked ambiguous"
		case "A12":
			status = "partial"
			reason = "Guide location width and phone displacement conflict; phone fields marked ambiguous"
		case "A13":
			status = "partial"
			reason = "Address base and data region; additional continuation retained"
		case "A16":
			status = "partial"
			reason = "Travelport+ UZ due/paid variant only; car, hotel, tour and SVC variants unresolved"
		case "A18":
			status = "unsupported"
			reason = "Discontinued by guide; original bytes retained"
		case "A27":
			status = "unresolved"
			reason = "Guide lists conflicting offsets for fee flags, fare indicator and totals; no offsets inferred"
		case "A29", "A30":
			status = "partial"
			reason = "Fixed EMD and coupon fields; ED-/EC- variable tails retained with unresolved status"
		case "A05":
			status = "partial"
			reason = "Waitlist base decoded; optional and alternative air layouts unresolved"
		case "A06":
			status = "partial"
			reason = "Repeated seat/smoking groups decoded; COG continuation and passenger identity unresolved"
		case "A10":
			status = "partial"
			reason = "Exchange association identifier decoded; old Apollo and newer exchange layouts require explicit disambiguation"
		case "A17":
			status = "partial"
			reason = "LeisureShopper fixed base decoded; cruise/tour optional structures remain unimplemented"
		case "A19":
			status = "partial"
			reason = "Miscellaneous document fixed base and ST/EB/R1/R2/R3/TN continuations decoded; ER/RI and other optional data unresolved"
		case "A20":
			status = "partial"
			reason = "SSR fixed base and FT continuation decoded; other host extensions remain raw"
		case "A21":
			status = "partial"
			reason = "Net remit base decoded; continuation variants not verified"
		case "A22":
			status = "partial"
			reason = "Seat base decoded; guide SCHX displacement overlaps SEN; repeated seat characteristics unresolved"
		case "A23":
			status = "partial"
			reason = "Refund ticket/date/name decoded; CDI displacement conflicts with C01; financial optional and large-fare variants unresolved"
		case "A24":
			status = "partial"
			reason = "Fare association, type and first construction line decoded; subsequent lines unresolved"
		case "A26":
			status = "unresolved"
			reason = "Guide SEC/DAT displacements conflict with illustrated 256-byte XML blocks; raw chunks retained"
		case "A28":
			status = "partial"
			reason = "Other fare level fixed base decoded; optional tax structures remain unimplemented"
		case "A31":
			status = "partial"
			reason = "Rail segment/leg/journey identity decoded; remaining rail fields and associated-remark structures unimplemented"
		case "A32":
			status = "partial"
			reason = "TASF passenger name and fee number decoded; remaining fee/tax/payment and additional name lines unimplemented"
		}
		out = append(out, Coverage{s, status, reason})
	}
	return out
}

func SectionCoverage(section string) Coverage {
	for _, c := range Registry() {
		if c.Section == section {
			return c
		}
	}
	return Coverage{section, "unsupported", "Unknown section identifier; raw data retained"}
}

// ParseStream uses the transmitted byte count T50SZE, never a header-looking
// continuation line, to separate records. An invalid boundary remains one failed
// source record, so no bytes disappear and no guessed records are manufactured.
func ParseStream(data []byte, name string) ([]*Result, error) {
	if len(data) == 0 || len(data) > 2<<20 {
		return nil, fmt.Errorf("source must contain 1 byte to 2 MiB")
	}
	var chunks [][]byte
	remaining := data
	for len(remaining) > 0 {
		if len(remaining) < 15 || !headerRE.Match(remaining) {
			chunks = nil
			break
		}
		n, e := strconv.Atoi(string(remaining[10:15]))
		if e != nil || n < 343 || n > len(remaining) {
			chunks = nil
			break
		}
		if n < len(remaining) && !headerRE.Match(remaining[n:]) {
			chunks = nil
			break
		}
		chunks = append(chunks, remaining[:n])
		remaining = remaining[n:]
	}
	ambiguous := len(chunks) == 0
	if ambiguous {
		chunks = [][]byte{data}
	}
	out := make([]*Result, 0, len(chunks))
	for _, chunk := range chunks {
		limit := 50000
		if len(chunk) >= 4 && string(chunk[2:4]) == "1G" {
			limit = 32767
		}
		if len(chunk) > limit {
			out = append(out, opaqueResult(chunk, name, "RECORD_LIMIT", fmt.Sprintf("Guide host limit is %d bytes; oversized source retained without field decoding", limit)))
			continue
		}
		// ParseMIR's inspection recovery is deliberately disabled here: only offset
		// zero is a record start. Internal header-like text remains source evidence.
		r, e := parseSingle(chunk, name)
		if e != nil {
			r = opaqueResult(chunk, name, "MAPPING_CONFLICT", "Overlapping or inconsistent field mappings; raw record retained without unsafe decoding")
		}
		if ambiguous {
			r.Issues = append(r.Issues, Issue{Severity: "error", Code: "UNRESOLVED_FRAMING", Message: "T50SZE does not establish reliable record boundaries; source retained as one failed record"})
		}
		out = append(out, r)
	}
	return out, nil
}

func opaqueResult(raw []byte, name, code, message string) *Result {
	r, _ := parseMIR([]byte{}, name, true)
	r.Raw = bytes.Clone(raw)
	r.Size = len(raw)
	r.Lines = splitLines(raw)
	r.Pieces = []Piece{{Start: 0, End: len(raw), Kind: "unknown", RawHex: fmt.Sprintf("%x", raw)}}
	hash := sha256.Sum256(raw)
	r.SHA256 = fmt.Sprintf("%x", hash)
	r.Issues = []Issue{{Severity: "error", Code: code, Message: message}}
	return r
}

func parseSingle(data []byte, name string) (*Result, error) {
	// Reuse the byte-preserving parser, with its record-start detection restricted
	// to the beginning by the stagingOnly flag.
	return parseMIR(data, name, true)
}

// MIR dates have an explicit two-digit year. The Go standard library pivot
// (69..99 => 1969..1999; 00..68 => 2000..2068) is explicit, never travel-derived.
func Date(value string) (string, error) {
	t, e := time.Parse("02Jan06", strings.ToUpper(strings.TrimSpace(value)))
	if e != nil {
		return "", e
	}
	return t.Format("2006-01-02"), nil
}

func (r *Result) Values(section string, item int) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	for _, f := range r.Fields {
		if f.Section != section || f.Item != item || (f.Status != "parsed" && f.Status != "blank") {
			continue
		}
		if seen[f.Label] {
			delete(out, f.Label)
			continue
		}
		seen[f.Label] = true
		out[f.Label] = trim(f.Raw)
	}
	return out
}

func (r *Result) ItemRaw(it Item) []byte {
	end := it.End
	for _, l := range r.Lines {
		if l.End == it.End {
			end += len(l.Ending)
			break
		}
	}
	return bytes.Clone(r.Raw[it.Start:end])
}

func extraSpecs() {
	extendedSpecs()
	add := func(s, label, name string, offset, length int, typ, page string) {
		specs = append(specs, Spec{Label: label, Name: name, Section: s, Offset: offset, Length: length, Type: typ, Page: page, Description: name})
	}
	for _, r := range []struct {
		label, name    string
		offset, length int
		typ            string
	}{{"SEC", "Section identifier", 0, 3, "B"}, {"FFP", "Frequent flyer passenger name", 3, 21, "A"}, {"FCC", "Frequent flyer carrier", 24, 2, "B"}, {"FSP", "Separator", 26, 1, "I"}, {"FFN", "Frequent flyer number", 27, 20, "B"}, {"CAI", "Cross accrual indicator", 47, 1, "I"}, {"CAA", "Cross accrual carrier list", 48, 30, "B"}} {
		add("A03", "A03"+r.label, r.name, r.offset, r.length, r.typ, "58")
	}
	// Guide pp. 217-222 specifies sequential fixed fields, followed by CR and
	// separately identified variable data. Offsets here exclude the terminating CR.
	layouts := map[string]string{
		"A29": "SEC:3:B PAX:2:N SEG:2:N SVC:4:A BKI:1:A EID:1:A RFI:1:B NAM:55:B EMD:13:N CNJ:13:N VCR:2:B CBA:3:A BAM:12:N/S CEA:3:A EAM:12:N/S CTX:3:A TAM:12:N/S CTA:3:A TLA:12:N/S BSR:13:B TYP:2:A AMT:12:N/S ACI:1:A APC:8:B",
		"A30": "SEC:3:B PAX:2:N SEG:2:N CPN:1:N EMD:13:N AIR:2:B OCC:3:A DAT:5:B REQ:2:N RFC:3:B FOR:30:B CUN:3:A UNT:3:N RAT:12:N/S QAL:1:A CBA:3:A BAM:12:N/S CEA:3:A EAM:12:N/S CTX:3:A TX1:8:B/S TC1:2:B TX2:8:B/S TC2:2:B TX3:8:B/S TC3:2:B CTA:3:A TLA:12:N/S ATK:13:N",
	}
	for s, layout := range layouts {
		offset := 0
		for _, token := range strings.Fields(layout) {
			p := strings.Split(token, ":")
			n, _ := strconv.Atoi(p[1])
			add(s, s+p[0], s+p[0], offset, n, p[2], "217-222")
			offset += n
		}
	}
}

func (p *parser) extraItem(it Item) bool {
	if p.extendedItem(it) {
		return true
	}
	first := p.result.Lines[it.Lines[0]-1]
	s := it.Section
	emit := func(label, name string, line Line, a, b, maximum int) {
		spec := Spec{Label: label, Name: name, Section: s, Type: "B/S", Page: "49,105"}
		p.emit(spec, line, a, min(b, len(line.Data)), it.Number, 0, "", "")
		if len(line.Data)-a > maximum {
			p.issue("FIELD_LIMIT", label+" exceeds guide maximum; overflow retained", line.Number)
		}
	}
	switch s {
	case "A00":
		p.structure(first, 0, 3)
		emit("A00CUS", "Customer identification remark", first, 3, 46, 43)
		for _, n := range it.Lines[1:] {
			l := p.result.Lines[n-1]
			if bytes.HasPrefix(l.Data, []byte("TO:")) {
				p.structure(l, 0, 3)
				emit("A00EAD", "Email address", l, 3, len(l.Data), len(l.Data))
			} else {
				p.issue("UNRESOLVED_CONTINUATION", "A00 client file or other optional line retained", n)
			}
		}
		return true
	case "A15":
		p.structure(first, 0, 3)
		p.emit(Spec{Label: "A15SEG", Name: "Associated segment number", Section: s, Type: "N", Page: "105"}, first, 3, 5, it.Number, 2, "", "")
		width := 88
		if p.host == "1V" {
			width = 70
		}
		emit("A15RMK", "Associated/unassociated remark", first, 5, 5+width, width)
		if len(it.Lines) > 1 {
			p.issue("UNRESOLVED_CONTINUATION", "Additional A15 lines retained", first.Number)
		}
		return true
	case "A18":
		p.issue("DISCONTINUED_SECTION", "A18 discontinued by guide; all bytes preserved", first.Number)
		return true
	}
	if s == "A29" || s == "A30" {
		if len(it.Lines) > 1 {
			p.issue("UNRESOLVED_CONTINUATION", s+" ED-/EC- variable data retained without invented field definitions", first.Number)
		}
	}
	return false
}
