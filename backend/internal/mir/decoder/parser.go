package decoder

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Line struct {
	Number, Start, End int
	Data, Ending       []byte
}
type Field struct {
	ID                               string
	Record                           int
	Section                          string
	Item                             int
	Label, Name, Raw, RawHex         string
	Value                            any
	Type                             string
	Start, End, Line, Column         int
	Status, Meaning, Reference, Note string
}
type Issue struct {
	Severity, Code, Message string
	Record, Line            int
}
type Item struct {
	Record     int
	Section    string
	Number     int
	Lines      []int
	Start, End int
}
type Record struct {
	Number               int
	Host                 string
	Start, End, Declared int
}
type Piece struct {
	Start, End            int
	Kind, FieldID, RawHex string
}
type Result struct {
	Filename                       string
	Size                           int
	SHA256, Encoding, DecodingNote string
	Raw                            []byte `json:"-"`
	Lines                          []Line
	Fields                         []Field
	Issues                         []Issue
	Items                          []Item
	Records                        []Record
	Pieces                         []Piece
}
type parser struct {
	result     *Result
	host       string
	record     int
	structures []Piece
}

var headerRE = regexp.MustCompile(`^T51[GV][0-9]{4}92[0-9]{10}`)
var sectionRE = regexp.MustCompile(`^A[0-9]{2}`)
var taxTagRE = regexp.MustCompile(`^(IT|TP|TN|ET|T[1-5]):`)

func latin(b []byte) string {
	r := make([]rune, len(b))
	for i, x := range b {
		r[i] = rune(x)
	}
	return string(r)
}
func trim(s string) string { return strings.Trim(s, " ") }
func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func splitLines(data []byte) []Line {
	var out []Line
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != 13 && data[i] != 10 {
			continue
		}
		end := i + 1
		if data[i] == 13 && end < len(data) && data[end] == 10 {
			end++
		}
		out = append(out, Line{len(out) + 1, start, i, data[start:i], data[i:end]})
		start = end
		i = end - 1
	}
	if start < len(data) || len(out) == 0 {
		out = append(out, Line{len(out) + 1, start, len(data), data[start:], nil})
	}
	return out
}
func encodingOf(data []byte) (string, string, bool) {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) || bytes.HasPrefix(data, []byte{0, 0, 0xfe, 0xff}) {
		return "utf-16/32", "Multibyte BOM; fixed-byte rules disabled.", false
	}
	b := data
	enc := "ascii"
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		enc = "utf-8-sig"
		b = b[3:]
	}
	ascii := true
	for _, x := range b {
		if x >= 128 {
			ascii = false
			break
		}
	}
	if ascii {
		return enc, "ASCII-compatible bytes; original bytes retained.", true
	}
	if utf8.Valid(data) {
		return "utf-8", "Multibyte UTF-8; fixed-byte rules disabled pending format confirmation.", false
	}
	return "latin-1", "Encoding undetermined; reversible byte view only; fixed-byte rules disabled.", false
}
func (p *parser) issue(code, message string, line int) {
	p.result.Issues = append(p.result.Issues, Issue{"warning", code, message, p.record, line})
}
func (p *parser) emit(s Spec, line Line, begin, end, item, expected int, status, note string) {
	begin = min(max(0, begin), len(line.Data))
	end = min(max(begin, end), len(line.Data))
	if begin == end {
		if expected > 0 {
			p.issue("MISSING_FIELD", s.Label+" has no available bytes", line.Number)
		}
		return
	}
	raw := line.Data[begin:end]
	text := latin(raw)
	v := trim(text)
	state := "parsed"
	if v == "" {
		state = "blank"
	}
	var value any = v
	codes := codeTables[s.Label]
	if s.Label == "A11TYP" {
		codes = paymentCodes[p.host]
	}
	_, hasBlank := codes[""]
	if len(codes) > 0 && (v != "" || hasBlank) {
		if meaning, ok := codes[v]; ok {
			value = map[string]string{"code": v, "meaning": meaning}
			state = "parsed"
		} else {
			state = "unknown_code"
			note += " Code not defined in implemented guide table."
		}
	}
	if expected > 0 && len(raw) < expected {
		state = "truncated"
		p.issue("TRUNCATED_FIELD", s.Label+" is short", line.Number)
	} else if s.Type == "N" && strings.TrimSpace(text) != "" && !digits(strings.TrimSpace(text)) && s.Label != "A04JTM" && s.Label != "A02TLD" {
		state = "invalid_type"
		note += " Expected numeric characters."
	}
	if status != "" {
		state = status
	}
	reference := "MIR User Guide, 2025-09-30, p. " + s.Page + ", " + s.Label
	f := Field{fmt.Sprintf("r%d:%s:%d:%s:%d", p.record, s.Section, item, s.Label, line.Start+begin), p.record, s.Section, item, s.Label, s.Name, text, hex.EncodeToString(raw), value, s.Type, line.Start + begin, line.Start + end, line.Number, begin + 1, state, s.Description, reference, note}
	p.result.Fields = append(p.result.Fields, f)
	if state != "parsed" && state != "blank" && state != "truncated" {
		p.issue("FIELD_"+strings.ToUpper(state), s.Label+": "+note, line.Number)
	}
}
func (p *parser) fixed(section string, line Line, item int, onlyIdentity bool) {
	for _, s := range fixedSpecs(section) {
		if onlyIdentity && s.Offset >= 5 {
			continue
		}
		if s.Offset >= len(line.Data) {
			p.issue("MISSING_FIELD", s.Label+" absent in short item", line.Number)
			continue
		}
		p.emit(s, line, s.Offset, s.Offset+s.Length, item, s.Length, "", "")
	}
}
func (p *parser) structure(line Line, a, b int) {
	a = min(a, len(line.Data))
	b = min(b, len(line.Data))
	if b > a {
		p.structures = append(p.structures, Piece{Start: line.Start + a, End: line.Start + b, Kind: "identifier"})
	}
}
func findTag(data []byte, offset int, options []Spec) (Spec, bool) {
	if offset > len(data) {
		return Spec{}, false
	}
	for _, s := range options {
		if bytes.HasPrefix(data[offset:], []byte(s.Tag)) {
			return s, true
		}
	}
	return Spec{}, false
}
func (p *parser) optional(section string, line Line, start, item int) {
	opts := optionalSpecs(section)
	cursor := start
	for cursor < len(line.Data) {
		s, ok := findTag(line.Data, cursor, opts)
		if !ok {
			if line.Data[cursor] == ' ' {
				cursor++
				continue
			}
			p.issue("UNKNOWN_OPTIONAL", fmt.Sprintf("%s optional bytes at column %d retained", section, cursor+1), line.Number)
			break
		}
		begin := cursor + len(s.Tag)
		end := min(begin+s.Length, len(line.Data))
		p.structure(line, cursor, begin)
		state, note := "", ""
		for i := begin; i < end; i++ {
			if _, ok := findTag(line.Data, i, opts); ok {
				end = i
				state = "ambiguous"
				note = "Recognized tag before documented maximum; verify variable boundary."
				if s.Label == "A02TLD" {
					note = "Guide length 8/type N conflicts with seven-character DDMMMYY. Boundary follows observed next tag."
				}
				break
			}
		}
		if s.Label == "A02DTE" {
			note = "Guide prints TD without colon; recognizes observed TD: spelling."
		}
		if section == "A04" && (s.Tag == "TK:" || s.Tag == "JT:") && p.host != "1G" {
			state = "not_applicable"
			note = "Travelport+ optional tag in other host record."
		}
		p.emit(s, line, begin, end, item, 0, state, note)
		cursor = end
	}
}
func (p *parser) taxes(line Line, start, item int) {
	cursor := start
	if cursor+6 <= len(line.Data) && regexp.MustCompile(`^[A-Z]{3}T[1-5]:`).Match(line.Data[cursor:]) {
		p.emit(specFor("A07CUR"), line, cursor, cursor+3, item, 3, "", "")
		cursor += 3
	}
	for cursor < len(line.Data) {
		m := taxTagRE.FindSubmatch(line.Data[cursor:])
		if m == nil {
			if trim(latin(line.Data[cursor:])) != "" {
				p.issue("UNKNOWN_TAX_LAYOUT", "Unimplemented A07 optional bytes retained", line.Number)
			}
			break
		}
		tag := string(m[1])
		p.structure(line, cursor, cursor+len(m[0]))
		cursor += len(m[0])
		if strings.HasPrefix(tag, "T") && len(tag) == 2 && tag[1] >= '1' && tag[1] <= '5' {
			n := int(tag[1] - '0')
			if n > 3 && p.host == "1G" {
				p.issue("HOST_DIFFERENCE", "Travelport+ documents three general tax boxes", line.Number)
			}
			p.emit(specFor(fmt.Sprintf("A07TT%d", n)), line, cursor, cursor+8, item, 8, "", "")
			p.emit(specFor(fmt.Sprintf("A07TC%d", n)), line, cursor+8, cursor+10, item, 2, "", "")
			cursor += 10
			continue
		}
		width := 8
		if tag == "ET" {
			width = 11
		}
		for n := 1; n <= 20 && cursor < len(line.Data); n++ {
			if taxTagRE.Match(line.Data[cursor:]) || trim(latin(line.Data[cursor:])) == "" {
				break
			}
			label := fmt.Sprintf("A07%s%d", tag, n)
			p.emit(specFor(label), line, cursor, cursor+width, item, width, "", "")
			p.emit(specFor(label+"C"), line, cursor+width, cursor+width+2, item, 2, "", "")
			cursor += width + 2
		}
	}
}
func (p *parser) item(it Item) {
	if p.extraItem(it) {
		return
	}
	lines := make([]Line, len(it.Lines))
	for i, n := range it.Lines {
		lines[i] = p.result.Lines[n-1]
	}
	first := lines[0]
	n := it.Number
	s := it.Section
	if len(fixedSpecs(s)) > 0 {
		alternative := s == "A04" && (bytes.Contains(first.Data[min(5, len(first.Data)):min(26, len(first.Data))], []byte("ARNK")) || trim(latin(first.Data[min(5, len(first.Data)):min(7, len(first.Data))])) == "*" || string(first.Data[min(7, len(first.Data)):min(10, len(first.Data))]) == "000" || string(first.Data[min(7, len(first.Data)):min(10, len(first.Data))]) == "   ")
		p.fixed(s, first, n, alternative)
		if alternative {
			p.issue("ALTERNATIVE_AIR_LAYOUT", "Surface/taxi or zero/blank carrier variant; remaining bytes retained", first.Number)
			return
		}
		switch s {
		case "A02":
			if len(first.Data) == 80 && p.host == "1G" && (first.Data[79] == 'Y' || first.Data[79] == 'N') {
				p.emit(specFor("A02FFN"), first, 79, 80, n, 1, "", "Guide displacement conflict; sole trailing Travelport+ Y/N recognized.")
			} else if len(first.Data) > 79 {
				p.issue("PASSENGER_TAIL", "Bytes after passenger base need investigation", first.Number)
			}
			for _, l := range lines[1:] {
				p.optional(s, l, 0, n)
			}
		case "A04":
			p.optional(s, first, 103, n)
			for _, l := range lines[1:] {
				p.optional(s, l, 0, n)
			}
		case "A07":
			p.taxes(first, 50, n)
			for _, l := range lines[1:] {
				p.taxes(l, 0, n)
			}
		case "A08":
			p.optional(s, first, 43, n)
			for _, l := range lines[1:] {
				p.optional(s, l, 0, n)
			}
		case "A09":
			for i, l := range lines {
				if i >= 5 {
					p.issue("FARE_CONSTRUCTION_TAIL", "Optional VAT/additional line retained without classification", l.Number)
					continue
				}
				begin := 0
				if i == 0 {
					begin = 6
				}
				p.emit(specFor(fmt.Sprintf("A09L5%d", i+1)), l, begin, len(l.Data), n, 0, "", "Physical line retained; ticket-dependent maximum not inferred.")
			}
		case "A11":
			width := 6
			if p.host == "1G" {
				width = 8
			}
			optionalStart := 49 + width
			state, note := "", ""
			if len(first.Data) > 52 {
				tail := first.Data[52:min(optionalStart, len(first.Data))]
				m := regexp.MustCompile(`A:|C:|P:|F:|H:|O:`).FindIndex(tail)
				if m != nil {
					optionalStart = 52 + m[0]
					width = max(0, optionalStart-49)
					state = "ambiguous"
					note = "Optional tag precedes host-specific guide boundary; verify approval/plan split."
					p.issue("PAYMENT_WIDTH", note, first.Number)
				}
			}
			p.emit(specFor("A11APC"), first, 46, 46+width, n, width, state, note)
			p.emit(specFor("A11PPO"), first, 46+width, optionalStart, n, 3, state, note)
			p.optional(s, first, optionalStart, n)
			for _, l := range lines[1:] {
				p.optional(s, l, 0, n)
			}
		case "A13":
			p.emit(specFor("A13DTA"), first, 5, min(228, len(first.Data)), n, 0, "", "")
		}
		return
	}
	switch s {
	case "A12":
		p.structure(first, 0, 3)
		p.emit(specFor("A12CTY"), first, 3, 6, n, 3, "", "")
		if len(first.Data) > 6 {
			p.emit(specFor("A12LOC"), first, 6, 7, n, 0, "ambiguous", "Only first location character mapped; guide permits 1/2 characters.")
			p.emit(specFor("A12PHN"), first, 7, len(first.Data), n, 0, "ambiguous", "Guide phone offset 70 conflicts with samples; inspection region only.")
		}
	case "A14":
		p.structure(first, 0, 3)
		limit := 64
		if p.host == "1G" {
			limit = 90
		}
		p.emit(specFor("A14RMK"), first, 3, min(len(first.Data), 3+limit), n, 0, "", "")
	case "A16":
		if len(first.Data) < 6 || string(first.Data[3:6]) != "UZ:" || p.host != "1G" {
			p.issue("UNSUPPORTED_LAYOUT", "Auxiliary variant not implemented; retained", first.Number)
			return
		}
		p.structure(first, 0, 3)
		for _, r := range []struct {
			label string
			a, b  int
		}{{"A16DPI", 3, 6}, {"A16DPP", 6, 8}, {"A16DIN", 8, 9}, {"A16DTE", 9, 14}} {
			p.emit(specFor(r.label), first, r.a, r.b, n, r.b-r.a, "", "")
		}
		if len(first.Data) > 14 && trim(string(first.Data[14:])) != "" {
			if first.Data[8] == 'T' {
				p.issue("TEXT_AUXILIARY_TAIL", "Text tail retained; due/paid fields not assumed", first.Number)
			} else {
				for _, r := range []struct {
					label string
					a, b  int
				}{{"A16DCR", 14, 17}, {"A16DPD", 17, 29}, {"A16DPA", 29, 30}, {"A16DPF", 30, 90}} {
					p.emit(specFor(r.label), first, r.a, r.b, n, 0, "", "")
				}
			}
		}
	default:
		p.issue("UNSUPPORTED_LAYOUT", s+" identified; layout not implemented; all bytes retained", first.Number)
	}
}
func (p *parser) recordLines(lines []Line, end int) {
	p.record++
	first := lines[0]
	p.host = string(first.Data[2:4])
	declared, _ := strconv.Atoi(string(first.Data[10:15]))
	p.result.Records = append(p.result.Records, Record{p.record, p.host, first.Start, end, declared})
	if declared != end-first.Start {
		p.issue("RECORD_SIZE", fmt.Sprintf("Header declares %d bytes; observed %d", declared, end-first.Start), first.Number)
	}
	starts := []int{0, 81, 145, 242, 294}
	widths := []int{80, 63, 96, 51, 48}
	body := min(5, len(lines))
	for i, start := range starts {
		if i >= len(lines) {
			p.issue("SHORT_HEADER", "Fewer than five header lines", first.Number)
			p.result.Issues[len(p.result.Issues)-1].Severity = "error"
			break
		}
		l := lines[i]
		if i > 0 && sectionRE.Match(l.Data) {
			body = i
			p.issue("SHORT_HEADER", "Section inside expected header", l.Number)
			p.result.Issues[len(p.result.Issues)-1].Severity = "error"
			break
		}
		if len(l.Data) != widths[i] {
			p.issue("HEADER_WIDTH", "Header line width differs from base layout", l.Number)
		}
		for _, s := range fixedSpecs("HEADER") {
			if s.Offset >= start && s.Offset < start+widths[i] {
				a := s.Offset - start
				p.emit(s, l, a, a+s.Length, 1, s.Length, "", "")
			}
		}
	}
	counts := map[string]int{}
	current := -1
	closed := true
	previous := ""
	for _, l := range lines[body:] {
		if len(l.Data) == 0 || bytes.Equal(l.Data, []byte{12}) {
			current = -1
			closed = true
			continue
		}
		if sectionRE.Match(l.Data) {
			s := string(l.Data[:3])
			if previous != "" && s < previous {
				p.issue("SECTION_ORDER", s+" follows "+previous, l.Number)
			}
			if previous != "" && s != previous && !closed {
				p.issue("MISSING_SECTION_SEPARATOR", "Recovered at "+s, l.Number)
			}
			previous = s
			counts[s]++
			p.result.Items = append(p.result.Items, Item{p.record, s, counts[s], []int{l.Number}, l.Start, l.End})
			current = len(p.result.Items) - 1
			closed = false
		} else if current >= 0 {
			p.result.Items[current].Lines = append(p.result.Items[current].Lines, l.Number)
			p.result.Items[current].End = l.End
		} else {
			p.issue("ORPHAN_LINE", "Line outside recognized item retained", l.Number)
		}
	}
	for _, it := range p.result.Items {
		if it.Record == p.record {
			p.item(it)
		}
	}
	h := p.result.values(p.record, "HEADER", 0)
	for i, label := range counters {
		if digits(h[label]) {
			n, _ := strconv.Atoi(h[label])
			if n != counts[counterSections[i]] {
				p.issue("ITEM_COUNT", fmt.Sprintf("%s expects %d %s; found %d", label, n, counterSections[i], counts[counterSections[i]]), 0)
			}
		}
	}
	fares, segments := map[string]bool{}, map[string]bool{}
	for _, f := range p.result.Fields {
		if f.Record != p.record {
			continue
		}
		if f.Label == "A07FSI" {
			fares[trim(f.Raw)] = true
		}
		if f.Label == "A04ITN" {
			segments[trim(f.Raw)] = true
		}
	}
	for _, f := range p.result.Fields {
		if f.Record != p.record || trim(f.Raw) == "" {
			continue
		}
		if (f.Label == "A02FIN" || f.Label == "A08FSI" || f.Label == "A09FSI") && !fares[trim(f.Raw)] {
			p.issue("FARE_LINK", f.Label+" refers to absent fare "+trim(f.Raw), f.Line)
		}
		if f.Label == "A08ITN" && !segments[trim(f.Raw)] {
			p.issue("SEGMENT_LINK", "Fare basis refers to absent A04 segment", f.Line)
		}
	}
	for _, pair := range [][2]string{{"T50IN7", "A10"}, {"T50IN9", "A09"}} {
		v := h[pair[0]]
		if (v == "Y" || v == "N") && ((v == "Y") != (counts[pair[1]] > 0)) {
			p.issue("PRESENCE_FLAG", pair[0]+" conflicts with presence of "+pair[1], 0)
		}
	}
}
func (r *Result) values(record int, section string, item int) map[string]string {
	out := map[string]string{}
	for _, f := range r.Fields {
		if f.Record == record && f.Section == section && (item == 0 || f.Item == item) {
			if _, ok := out[f.Label]; !ok {
				out[f.Label] = trim(f.Raw)
			}
		}
	}
	return out
}
func (p *parser) ledger() error {
	spans := append([]Piece{}, p.structures...)
	for _, f := range p.result.Fields {
		spans = append(spans, Piece{Start: f.Start, End: f.End, Kind: "field", FieldID: f.ID})
	}
	for _, l := range p.result.Lines {
		if len(l.Ending) > 0 {
			spans = append(spans, Piece{Start: l.End, End: l.End + len(l.Ending), Kind: "line_ending"})
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	cursor := 0
	gap := func(a, b int) {
		kind := "unknown"
		data := p.result.Raw[a:b]
		if len(bytes.Trim(data, " ")) == 0 {
			kind = "padding"
		} else if len(bytes.Trim(data, "\x0c")) == 0 {
			kind = "framing"
		}
		p.result.Pieces = append(p.result.Pieces, Piece{a, b, kind, "", hex.EncodeToString(data)})
	}
	for _, s := range spans {
		if s.Start < cursor {
			return errors.New("overlapping physical source mappings")
		}
		if s.Start > cursor {
			gap(cursor, s.Start)
		}
		s.RawHex = hex.EncodeToString(p.result.Raw[s.Start:s.End])
		p.result.Pieces = append(p.result.Pieces, s)
		cursor = s.End
	}
	if cursor < len(p.result.Raw) {
		gap(cursor, len(p.result.Raw))
	}
	return nil
}
func ParseMIR(data []byte, name string) (*Result, error) { return parseMIR(data, name, false) }
func parseMIR(data []byte, name string, stagingOnly bool) (*Result, error) {
	raw := bytes.Clone(data)
	hash := sha256.Sum256(raw)
	enc, note, eligible := encodingOf(raw)
	r := &Result{Filename: name, Size: len(raw), SHA256: hex.EncodeToString(hash[:]), Encoding: enc, DecodingNote: note, Raw: raw, Lines: splitLines(raw), Fields: []Field{}, Issues: []Issue{}, Items: []Item{}, Records: []Record{}, Pieces: []Piece{}}
	p := &parser{result: r}
	controls := 0
	variant := false
	for i, b := range raw {
		badFF := b == 12 && len(bytes.Trim(raw[i+1:], "\r\n\x0c")) > 0
		if (b < 32 && b != 10 && b != 12 && b != 13) || b == 127 || badFF {
			if controls < 20 {
				p.issue("CONTROL_CHARACTER", fmt.Sprintf("Control byte %02X at offset %d preserved", b, i), 0)
			}
			controls++
		}
		if b == 10 {
			variant = true
		}
	}
	if controls > 20 {
		p.issue("CONTROL_LIMIT", "Further control notices suppressed", 0)
	}
	if variant {
		p.issue("LINE_ENDING_VARIANT", "LF/CRLF variant; original offsets retained", 0)
	}
	if eligible {
		if enc == "utf-8-sig" {
			r.Lines[0].Start += 3
			r.Lines[0].Data = r.Lines[0].Data[3:]
		}
		var starts []int
		for i, l := range r.Lines {
			if headerRE.Match(l.Data) && (!stagingOnly || i == 0) {
				starts = append(starts, i)
			}
		}
		if len(starts) == 0 {
			p.issue("NO_HEADER", "No supported MIR92 header; all bytes inspectable", 0)
			r.Issues[len(r.Issues)-1].Severity = "error"
		}
		for i, start := range starts {
			next := len(r.Lines)
			end := len(raw)
			if i+1 < len(starts) {
				next = starts[i+1]
				end = r.Lines[next].Start
			}
			p.recordLines(r.Lines[start:next], end)
		}
	} else {
		p.issue("ENCODING_UNCERTAIN", note, 0)
		r.Issues[len(r.Issues)-1].Severity = "error"
	}
	if e := p.ledger(); e != nil {
		return nil, e
	}
	return r, nil
}
func Reconstruct(r *Result) ([]byte, error) {
	var out []byte
	cursor := 0
	for _, s := range r.Pieces {
		if s.Start != cursor {
			return nil, errors.New("byte ledger gap/overlap")
		}
		b, e := hex.DecodeString(s.RawHex)
		if e != nil || len(b) != s.End-s.Start {
			return nil, errors.New("byte ledger length/hex mismatch")
		}
		out = append(out, b...)
		cursor = s.End
	}
	if cursor != r.Size {
		return nil, errors.New("byte ledger incomplete")
	}
	return out, nil
}
