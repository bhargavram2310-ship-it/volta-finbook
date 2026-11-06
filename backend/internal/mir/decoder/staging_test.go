package decoder

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic fixtures are constructed for testing guide layouts. They are not
// actual Travelport files and do not contain real passenger information.
func synthetic(body, host, ending string) []byte {
	first := []byte(strings.Repeat(" ", 80))
	copy(first, []byte("T5"+host+"7733920000000001"))
	copy(first[20:], "05NOV26")
	copy(first[61:], "10DEC26")
	h2 := []byte(strings.Repeat(" ", 63))
	copy(h2[17:], "SYN001")
	data := bytes.Join([][]byte{first, h2, []byte(strings.Repeat(" ", 96)), []byte(strings.Repeat(" ", 51)), []byte(strings.Repeat("0", 48)), {}, []byte(body)}, []byte(ending))
	copy(data[10:15], fmt.Sprintf("%05d", len(data)))
	return data
}
func one(t *testing.T, b []byte) *Result {
	t.Helper()
	rs, e := ParseStream(b, "SYNTHETIC.MIR")
	if e != nil || len(rs) != 1 {
		t.Fatalf("parse failed: %v", e)
	}
	rebuilt, e := Reconstruct(rs[0])
	if e != nil || !bytes.Equal(b, rebuilt) {
		t.Fatal("source bytes changed")
	}
	return rs[0]
}
func fieldValue(r *Result, label string) Field {
	for _, f := range r.Fields {
		if f.Label == label {
			return f
		}
	}
	return Field{}
}
func TestEveryDocumentedSection(t *testing.T) {
	registry := Registry()
	if len(registry) != 32 {
		t.Fatalf("registry size %d", len(registry))
	}
	for _, c := range registry {
		t.Run(c.Section, func(t *testing.T) {
			raw := synthetic(c.Section+"\r\r", "1G", "\r")
			r := one(t, raw)
			if len(r.Items) != 1 || r.Items[0].Section != c.Section {
				t.Fatal("section discarded")
			}
			if len(r.ItemRaw(r.Items[0])) < 3 {
				t.Fatal("raw item unavailable")
			}
			if c.Status == "" || c.Reason == "" {
				t.Fatal("missing decoder coverage")
			}
		})
	}
}
func TestGuideHeaderAndIndependentDates(t *testing.T) {
	for _, ending := range []string{"\r", "\r\n", "\n"} {
		r := one(t, synthetic("", "1G", ending))
		if fieldValue(r, "T50DTE").Raw != "05NOV26" || fieldValue(r, "T50DFT").Raw != "10DEC26" {
			t.Fatal("date bytes shifted")
		}
		date, e := Date(fieldValue(r, "T50DTE").Raw)
		if e != nil || date != "2026-11-05" {
			t.Fatalf("date %s %v", date, e)
		}
	}
	if _, e := Date("31FEB26"); e == nil {
		t.Fatal("accepted invalid date")
	}
}
func TestGuidePassengerAndTaxLayouts(t *testing.T) {
	pax := []byte(strings.Repeat(" ", 79))
	copy(pax, "A02SYNTHETIC/PERSON")
	copy(pax[48:], "1234567890")
	copy(pax[75:], "07")
	fare := []byte(strings.Repeat(" ", 50))
	copy(fare, "A0707USD")
	copy(fare[8:], "      100.00")
	copy(fare[20:], "USD      120.00")
	r := one(t, synthetic(string(pax)+"\rEN:SYNTHETIC/EXPANDED NAME\r\r"+string(fare)+"T1:   20.00US\r\r", "1G", "\r"))
	if r.Values("A02", 1)["A02FIN"] != "07" || r.Values("A02", 1)["A02EXN"] != "SYNTHETIC/EXPANDED NAME" {
		t.Fatal("optional passenger layout")
	}
	if r.Values("A07", 1)["A07TBF"] != "100.00" || r.Values("A07", 1)["A07TT1"] != "20.00" || r.Values("A07", 1)["A07TC1"] != "US" {
		t.Fatal("fare/tax boundaries")
	}
}
func TestOptionalVariableContinuationsAndRepeatedItems(t *testing.T) {
	body := "A00SYNTHETIC CUSTOMER\rTO:a@example.test\r\rA15" + "01first remark\rA15" + "00second remark\r\rA99UNKNOWN\rcontinued bytes\r\rA18DISCONTINUED\r\r"
	r := one(t, synthetic(body, "1G", "\r"))
	if r.Values("A00", 1)["A00EAD"] != "a@example.test" || r.Values("A15", 2)["A15RMK"] != "second remark" {
		t.Fatal("continuation/repeat decoding")
	}
	if len(r.Items) != 5 || len(r.Items[3].Lines) != 2 {
		t.Fatal("unknown item continuation lost")
	}
}
func TestApolloAndTravelportPaymentWidths(t *testing.T) {
	for _, host := range []string{"1G", "1V"} {
		code := "ABC123"
		if host == "1G" {
			code = "ABC12345"
		}
		body := "A11" + strings.Repeat(" ", 43) + code + "XYZP:01\r\r"
		r := one(t, synthetic(body, host, "\r"))
		if fieldValue(r, "A11APC").Raw != code || fieldValue(r, "A11PGR").Raw != "01" {
			t.Fatal("host-specific payment boundaries")
		}
	}
}
func TestEMDFixedBoundariesAndRepeatedSeats(t *testing.T) {
	emd := []byte(strings.Repeat(" ", 226))
	copy(emd, "A290102SVC ASASYNTHETIC")
	copy(emd[69:], "1234567890123")
	r := one(t, synthetic(string(emd)+"\rED-/ER-TEST\r\rA060112AN13BY\r\r", "1G", "\r"))
	if fieldValue(r, "A29EMD").Start-fieldValue(r, "A29SEC").Start != 69 || fieldValue(r, "A29EMD").Raw != "1234567890123" {
		t.Fatal("guide EMD fixed offset")
	}
	count := 0
	for _, f := range r.Fields {
		if f.Label == "A06SEN" {
			count++
		}
	}
	if count != 2 {
		t.Fatal("repeating seat groups lost")
	}
}
func TestReliableStreamsAndHeaderLikeText(t *testing.T) {
	a := synthetic("A14FIRST\r\r", "1G", "\r")
	b := synthetic("A14SECOND\r\r", "1V", "\r")
	rs, e := ParseStream(append(bytes.Clone(a), b...), "SYNTHETIC-STREAM.MIR")
	if e != nil || len(rs) != 2 {
		t.Fatal("size-framed stream")
	}
	body := "A14TEXT\rT51G7733920034300001" + strings.Repeat(" ", 60) + "\r\r"
	r := one(t, synthetic(body, "1G", "\r"))
	if len(r.Records) != 1 {
		t.Fatal("header-like continuation split record")
	}
	bad := bytes.Clone(a)
	copy(bad[10:15], "99999")
	r = one(t, append(bad, b...))
	found := false
	for _, i := range r.Issues {
		if i.Code == "UNRESOLVED_FRAMING" {
			found = true
		}
	}
	if !found {
		t.Fatal("guessed stream boundary")
	}
}
func TestTruncatedMalformedLargeAndRandomBytes(t *testing.T) {
	r := one(t, synthetic("A02SHORT\r\r", "1G", "\r"))
	if fieldValue(r, "A02NME").Status != "truncated" {
		t.Fatal("truncated field accepted")
	}
	for _, host := range []string{"1G", "1V"} {
		limit := 32767
		if host == "1V" {
			limit = 50000
		}
		base := synthetic("", "1G", "\r")
		body := strings.Repeat("Z", limit-len(base))
		r := one(t, synthetic(body, host, "\r"))
		if len(r.Raw) != limit {
			t.Fatal("large record fixture size")
		}
		for _, i := range r.Issues {
			if i.Code == "RECORD_LIMIT" {
				t.Fatal("within-limit record rejected")
			}
		}
	}
	rng := rand.New(rand.NewSource(19))
	for n := 0; n < 200; n++ {
		b := make([]byte, rng.Intn(4000)+1)
		rng.Read(b)
		r := one(t, b)
		if len(r.Raw) != len(b) {
			t.Fatal("malformed source lost")
		}
	}
}
func TestAABYXGALRegression(t *testing.T) {
	path := os.Getenv("MIR_REGRESSION_FILE")
	if path == "" {
		path = filepath.Join("..", "..", "..", "..", "..", "IDC-MIR-Reader", "reference", "sample_files", "AABYXGAL.MIR")
	}
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		t.Skip("private AABYXGAL.MIR not available")
	}
	if e != nil {
		t.Fatal(e)
	}
	r := one(t, raw)
	if len(r.Records) != 1 {
		t.Fatal("regression framing")
	}
	segments := 0
	for _, it := range r.Items {
		if it.Section == "A04" {
			segments++
		}
	}
	if segments != 5 || r.Values("A07", 1)["A07TBF"] != "108815" {
		t.Fatal("existing parser regression")
	}
}

func TestHostSizeLimitPrecedesFieldDecoding(t *testing.T) {
	for _, host := range []string{"1G", "1V"} {
		limit := 32767
		if host == "1V" {
			limit = 50000
		}
		base := synthetic("", host, "\r")
		raw := synthetic(strings.Repeat("Z", limit+1-len(base)), host, "\r")
		r := one(t, raw)
		if len(r.Fields) != 0 || len(r.Issues) != 1 || r.Issues[0].Code != "RECORD_LIMIT" {
			t.Fatal("oversized source entered field decoder")
		}
	}
}
