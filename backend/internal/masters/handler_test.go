package masters

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMasterEndpointsRequireAuthentication(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		w := httptest.NewRecorder()
		NewHandler(nil).ServeHTTP(w, httptest.NewRequest(method, "/api/v1/masters/airlines", strings.NewReader(`{}`)))
		if w.Code != 401 {
			t.Fatalf("%s: %d", method, w.Code)
		}
	}
}

func TestMasterInputRejectsUnsafeAndInexactValues(t *testing.T) {
	cases := []struct {
		field Field
		value string
	}{
		{number("rate", "Rate", "rate", true), "0"},
		{number("rate", "Rate", "rate", true), "1.0000000000001"},
		{number("credit_limit", "Credit limit", "decimal", true), "1.0000001"},
		{number("rate_percent", "Percent", "percent", true), "100.000001"},
		{number("decimal_places", "Precision", "precision", true), "7"},
		{number("version", "Version", "integer", true), "0"},
		{object("basis", "Basis"), "[]"},
		{ref("company_id", "Company", "company", true), "' OR 1=1"},
		{date("effective_date", "Date", true), "2026-02-30"},
	}
	for _, item := range cases {
		input := mutation{Values: map[string]string{item.field.Name: item.value}}
		if normalize(Definition{Fields: []Field{item.field}}, &input) == nil {
			t.Errorf("accepted %s=%s", item.field.Name, item.value)
		}
	}
	input := mutation{Values: map[string]string{"unexpected": "value"}}
	if normalize(Definition{}, &input) == nil {
		t.Fatal("accepted unknown column")
	}
}

func TestMasterMutationRejectsCrossSiteAndTrailingPayload(t *testing.T) {
	for _, body := range []string{`{} {}`, `{"values":{},"extra":true}`} {
		r := httptest.NewRequest("POST", "http://localhost/api/v1/masters/airlines", strings.NewReader(body))
		w := httptest.NewRecorder()
		if decodeMutation(w, r, &mutation{}) || w.Code != 400 {
			t.Fatal("accepted invalid body")
		}
	}
	r := httptest.NewRequest("DELETE", "http://localhost/api/v1/masters/airlines/id", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	if decodeMutation(w, r, &mutation{}) || w.Code != 403 {
		t.Fatal("accepted foreign origin")
	}
}
