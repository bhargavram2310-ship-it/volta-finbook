package mir

import (
	"testing"
	"voltafinbook/backend/internal/mir/decoder"
)

func TestCaptureDraftMapsOnlySelectedPassengerAndLinkedFare(t *testing.T) {
	parsed, err := decoder.ParseStream(syntheticStageRecord(), "SYNTHETIC.MIR")
	if err != nil || len(parsed) != 1 {
		t.Fatal("fixture parse failed")
	}
	draft := buildCaptureDraft(parsed[0], 2, []draftItem{{section: "A07", number: 1}}, nil)
	for key, want := range map[string]string{"ticketNumber": "1231234567891", "passengerName": "SYNTHETIC/TWO", "pnr": "SYN001", "ticketDate": "04NOV26", "basicFare": "100.00", "mirTotalFare": "120.00", "basicCurrency": "USD", "fareCurrency": "USD"} {
		field, ok := draft.Fields[key]
		if !ok || field.Value != want || len(field.Lines) == 0 || len(field.FieldIDs) == 0 {
			t.Fatalf("%s not mapped with evidence: %+v", key, field)
		}
	}
	for _, key := range []string{"billTo", "supplier", "clientTotal", "supplierTotal", "sector"} {
		if _, ok := draft.Fields[key]; ok {
			t.Fatalf("unlinked or commercial value %s was inferred", key)
		}
	}
	if len(draft.RawLines) == 0 || len(draft.SHA256) != 64 {
		t.Fatal("source evidence missing")
	}
	unlinked := buildCaptureDraft(parsed[0], 1, nil, nil)
	if _, ok := unlinked.Fields["mirTotalFare"]; ok {
		t.Fatal("fare assigned without a passenger relationship")
	}
}

func TestCaptureDraftLeavesAmbiguousAndUnsupportedValuesBlank(t *testing.T) {
	parsed, err := decoder.ParseStream(syntheticStageRecord(), "SYNTHETIC.MIR")
	if err != nil {
		t.Fatal(err)
	}
	r := parsed[0]
	for i, f := range r.Fields {
		if f.Label == "A02NME" && f.Item == 1 {
			r.Fields = append(r.Fields, f)
		}
		if f.Label == "A07TTA" {
			r.Fields[i].Raw = "USD 120.00"
		}
	}
	draft := buildCaptureDraft(r, 1, []draftItem{{section: "A07", number: 1}}, nil)
	if _, ok := draft.Fields["passengerName"]; ok {
		t.Fatal("ambiguous passenger name chosen")
	}
	if _, ok := draft.Fields["mirTotalFare"]; ok {
		t.Fatal("unsupported monetary representation accepted")
	}
}
