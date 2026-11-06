package decoder

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed go_schema/*.json
var assets embed.FS

type Spec struct {
	Label       string `json:"label"`
	Name        string `json:"name"`
	Section     string `json:"section"`
	Offset      int    `json:"offset"`
	Length      int    `json:"length"`
	Type        string `json:"data_type"`
	Page        string `json:"page"`
	Tag         string `json:"tag"`
	Description string `json:"description"`
}

var specs []Spec
var codeTables, paymentCodes map[string]map[string]string
var counters = []string{"T50CRN", "T50CPN", "T50PGN", "T50FFN", "T50ARN", "T50WLN", "T50SDN", "T50FBN", "T50EXC", "T50PYN", "T50PHN", "T50ADN", "T50MSN", "T50RRN", "T50AXN", "T50LSN"}
var counterSections = []string{"A00", "A01", "A02", "A03", "A04", "A05", "A06", "A07", "A10", "A11", "A12", "A13", "A14", "A15", "A16", "A17"}

func init() {
	read := func(path string, v any) {
		b, e := assets.ReadFile(path)
		if e != nil {
			panic(e)
		}
		if e = json.Unmarshal(b, v); e != nil {
			panic(e)
		}
	}
	read("go_schema/fields.json", &specs)
	read("go_schema/code_tables.json", &codeTables)
	read("go_schema/payment_codes.json", &paymentCodes)
	add := func(section, label, name string, offset, length int, typ, page, tag string) {
		specs = append(specs, Spec{label, name, section, offset, length, typ, page, tag, name})
	}
	for _, section := range []string{"A01", "A02", "A04", "A07", "A08", "A09", "A11", "A13"} {
		add(section, section+"SEC", "Section identifier", 0, 3, "B", "48", "")
	}
	for n := 1; n <= 5; n++ {
		add("HEADER", fmt.Sprintf("T50%dTX", n), fmt.Sprintf("Header tax %d amount", n), 164+(n-1)*10, 8, "B", "36", "")
		add("HEADER", fmt.Sprintf("T50%dTC", n), fmt.Sprintf("Header tax %d code", n), 172+(n-1)*10, 2, "B", "36", "")
	}
	for i, label := range counters {
		add("HEADER", label, "Count: "+counterSections[i], 294+3*i, 3, "N", "45â€“46", "")
	}
	names := []string{"Retransmission", "Manual pricing", "360 Fares pricing", "One fare for all passengers", "Split ticketing issued", "ATB / ticketing type", "Exchange information present", "Conjunction required", "Fare construction present", "Seat data", "TINS present", "Issuance command", "SITI/SOTO obsolete", "Travel advisory", "Group manager data", "Direct booking", "Itinerary geography"}
	for i, name := range names {
		add("HEADER", fmt.Sprintf("T50IN%d", i+1), name, 242+i, 1, "A", "38â€“43", "")
	}
	for n := 1; n <= 5; n++ {
		add("A09", fmt.Sprintf("A09L5%d", n), fmt.Sprintf("Fare construction line %d", n), -1, 61, "B", "83â€“85", "")
		add("A07", fmt.Sprintf("A07TT%d", n), fmt.Sprintf("Tax box %d amount", n), -1, 8, "B/S", "72â€“73", fmt.Sprintf("T%d:", n))
		add("A07", fmt.Sprintf("A07TC%d", n), fmt.Sprintf("Tax box %d code", n), -1, 2, "B", "73", "")
	}
	for _, prefix := range []string{"IT", "TP", "TN", "ET"} {
		width := 8
		if prefix == "ET" {
			width = 11
		}
		for n := 1; n <= 20; n++ {
			label := fmt.Sprintf("A07%s%d", prefix, n)
			add("A07", label, prefix+" tax amount", -1, width, "B/S", "73â€“79", "")
			add("A07", label+"C", prefix+" tax code", -1, 2, "B", "73â€“79", "")
		}
	}
	codeTables["T50TRC"] = map[string]string{"1G": "Travelport+", "1V": "Travelport Apollo"}
	yesno := map[string]string{"Y": "Yes", "N": "No"}
	for _, n := range []int{1, 2, 3, 4, 5, 7, 8, 9, 11, 14, 15, 16} {
		codeTables[fmt.Sprintf("T50IN%d", n)] = yesno
	}
	codeTables["T50IN6"] = map[string]string{"0": "Non ATB", "1": "ATB", "2": "Reprinted ATB", "3": "Mini itinerary", "4": "Boarding pass only", "5": "Electronic ticketing", "6": "Electronic coupon print", "7": "ATB2", "8": "Other"}
	codeTables["T50IN10"] = map[string]string{"A": "No seat data", "B": "Seat data, no boarding pass", "C": "Boarding pass only", "D": "Ticket and boarding pass"}
	codeTables["T50IN12"] = map[string]string{"H": "Regular MIR (HB/TKP)", "X": "Dual MIR", "N": "Non-ticketing", "J": "Non-faring", "A": "Non-accounting", "B": "Accounting (Apollo)", "R": "Refund", "V": "Void", "U": "Unvoid", "S": "Spoiled", "C": "Cancelled refund", "D": "EMD refund", "I": "EMD exchange", "F": "EMD issue", "Z": "EMD void", "G": "Global MIR", "L": "Electronic ticket (Apollo)"}
	codeTables["T50IN17"] = map[string]string{"X": "International", "T": "Transborder US/Canada/Mexico", "": "Domestic"}
	var indicatorCodes map[string]map[string]string
	read("go_schema/header_indicator_codes.json", &indicatorCodes)
	for label, codes := range indicatorCodes {
		codeTables[label] = codes
	}
	extraSpecs()
}
func specFor(label string) Spec {
	for _, s := range specs {
		if s.Label == label {
			return s
		}
	}
	panic("Missing reviewed specification: " + label)
}
func fixedSpecs(section string) []Spec {
	var out []Spec
	for _, s := range specs {
		if s.Section == section && s.Offset >= 0 {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Offset < out[j].Offset })
	return out
}
func optionalSpecs(section string) []Spec {
	var out []Spec
	for _, s := range specs {
		if s.Section == section && s.Tag != "" {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Tag) > len(out[j].Tag) })
	return out
}
