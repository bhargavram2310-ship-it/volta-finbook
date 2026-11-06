package decoder

import (
	"bytes"
	"strconv"
	"strings"
)

// Transcribed from supplied MIR User Guide 2025-09-30, listed pages. Only
// unambiguous fixed bases are included. Conflicting layouts remain partial.
func extendedSpecs() {
	layouts := map[string]struct{ page, layout string }{
		"A05": {"68-70", "SEC:3:B ITN:2:N CDE:2:B NUM:3:N NME:12:B FLT:4:N CLS:2:B STS:2:A DTE:5:B TME:5:B ARV:5:B IND:1:N OCC:3:A OCN:13:A DCC:3:A DCN:13:A SVC:4:A"},
		"A10": {"85,90", "SEC:3:B EXI:2:N"},
		"A17": {"150-151", "SEC:3:B SEG:2:N CTI:1:A STT:2:A TSD:7:B NUM:2:N TVC:3:B PSE:1:A PST:1:A PSD:7:B NTS:3:N PED:7:B TGW:5:A DEP:5:A RET:5:A CUR:3:B PKD:30:B BKD:7:B"},
		"A19": {"167-168", "SEC:3:B MPD:3:B MTO:40:B MAT:15:B"},
		"A20": {"169-171", "SEC:3:B SEG:2:N PAX:2:N IND:3:A CDE:4:A STS:2:A REQ:2:N AIR:2:B OCC:3:A DCC:3:A FLT:4:N DAT:5:B NAM:55:A"},
		"A21": {"173", "SEC:3:B NFC:3:A NRT:12:N NAI:20:B NVC:20:B ITC:12:N NFR:3:B"},
		"A22": {"174-175", "SEC:3:B SEG:2:N PAS:2:N STT:1:B SEN:6:B"},
		"A23": {"176-177", "SEC:3:B TKT:14:N CON:1:S RTK:14:N DOI:7:B INV:9:B NME:33:A"},
		"A24": {"193", "SEC:3:B FSI:2:N TY5:1:N"},
		"A28": {"212-213", "SEC:3:B FSI:2:N LVL:1:A CRB:3:A TBF:12:N/S CRT:3:A TTA:12:N/S CRE:3:A EQV:12:N/S"},
		"A31": {"228-231", "SEC:3:B NME:2:N SEG:1:A NUM:2:N LEG:1:A LID:2:N JTP:1:A DOM:1:A B01:1:B"},
		"A32": {"238", "SEC:3:B NME:55:A/S SFN:13:N"},
	}
	for section, entry := range layouts {
		offset := 0
		for _, token := range strings.Fields(entry.layout) {
			parts := strings.Split(token, ":")
			n, _ := strconv.Atoi(parts[1])
			label := section + parts[0]
			specs = append(specs, Spec{Label: label, Name: label, Section: section, Offset: offset, Length: n, Type: parts[2], Page: entry.page, Description: "Guide-defined " + label})
			offset += n
		}
	}
}

func (p *parser) extendedItem(it Item) bool {
	s := it.Section
	first := p.result.Lines[it.Lines[0]-1]
	emit := func(label, name string, l Line, a, b int, typ, page string) {
		p.emit(Spec{Label: label, Name: name, Section: s, Type: typ, Page: page}, l, a, b, it.Number, 0, "", "")
	}
	// A06 seat/smoking groups repeat within one item, and have no passenger
	// identifier. Their physical order is retained without asserting a PAX link.
	if s == "A06" {
		p.structure(first, 0, 3)
		p.emit(Spec{Label: "A06SEG", Name: "Itinerary index", Section: s, Type: "N", Page: "70"}, first, 3, 5, it.Number, 2, "", "")
		for a := 5; a < len(first.Data); a += 4 {
			p.emit(Spec{Label: "A06SEN", Name: "Seat number", Section: s, Type: "B", Page: "70"}, first, a, a+3, it.Number, 3, "", "")
			p.emit(Spec{Label: "A06SMK", Name: "Smoking indicator", Section: s, Type: "A", Page: "70"}, first, a+3, a+4, it.Number, 1, "", "")
		}
		if len(it.Lines) > 1 {
			p.issue("UNRESOLVED_CONTINUATION", "A06 COG seat continuation retained without passenger inference", first.Number)
		}
		return true
	}
	if s == "A24" {
		p.fixed(s, first, it.Number, false)
		emit("A24L51", "First line of other fare construction", first, 6, min(67, len(first.Data)), "B", "193")
		if len(it.Lines) > 1 {
			p.issue("UNRESOLVED_CONTINUATION", "A24 additional fare construction lines retained", first.Number)
		}
		return true
	}
	if s == "A19" || s == "A20" {
		p.fixed(s, first, it.Number, false)
		for _, n := range it.Lines[1:] {
			l := p.result.Lines[n-1]
			var label, name string
			maximum := 0
			if s == "A20" && bytes.HasPrefix(l.Data, []byte("FT:")) {
				label = "A20TXT"
				name = "SSR free text"
				maximum = 180
			}
			if s == "A19" {
				for _, tag := range []struct {
					tag, label string
					maximum    int
				}{{"ST:", "A19MST", 52}, {"EB:", "A19MEB", 54}, {"R1:", "A19MR1", 46}, {"R2:", "A19MR2", 54}, {"R3:", "A19MR3", 106}, {"TN:", "A19MTN", 13}} {
					if bytes.HasPrefix(l.Data, []byte(tag.tag)) {
						label = tag.label
						name = tag.label
						maximum = tag.maximum
						break
					}
				}
			}
			if label == "" {
				p.issue("UNRESOLVED_CONTINUATION", s+" unimplemented optional line retained", n)
				continue
			}
			p.structure(l, 0, 3)
			emit(label, name, l, 3, min(3+maximum, len(l.Data)), "B/S", "167-171")
			if len(l.Data) > maximum+3 {
				p.issue("FIELD_LIMIT", label+" exceeds guide maximum; overflow retained", n)
			}
		}
		return true
	}
	if s == "A22" {
		p.fixed(s, first, it.Number, false)
		p.issue("UNRESOLVED_LAYOUT", "A22SCHX offset 12 overlaps A22SEN bytes 8..13; characteristics and repeated seat structure retained", first.Number)
		return true
	}
	if s == "A10" || s == "A17" || s == "A23" || s == "A28" || s == "A31" || s == "A32" {
		p.fixed(s, first, it.Number, false)
		p.issue("UNRESOLVED_LAYOUT", s+" base decoded; remaining host/layout-dependent optional data retained", first.Number)
		return true
	}
	return false
}
