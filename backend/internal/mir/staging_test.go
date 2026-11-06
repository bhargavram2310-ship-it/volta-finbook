package mir

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestStageRoutesRequireAuthentication(t *testing.T) {
	h := NewStageHandler(nil)
	for _, path := range []string{"/api/v1/mir/capture/settings", "/api/v1/mir/capture/1/details?record_id=1", "/api/v1/mir/capture/1/sale?record_id=1"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatalf("unauthenticated capture detail accessible: %s", path)
		}
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/api/v1/mir/capture/1/draft?record_id=1"}, {"POST", "/api/v1/mir/pull"}, {"GET", "/api/v1/mir/capture"}, {"POST", "/api/v1/mir/import"}, {"GET", "/api/v1/mir/records"}, {"GET", "/api/v1/mir/records/1"}, {"POST", "/api/v1/mir/records/1/reprocess"}, {"GET", "/api/v1/mir/batches"}, {"GET", "/api/v1/mir/batches/1"}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != 401 {
			t.Fatalf("%s status %d", tc.path, w.Code)
		}
	}
}

func TestCaptureRequiresValidBoundedDateFilters(t *testing.T) {
	for _, query := range []string{"", "from_date=2026-11-05", "from_date=2026-11-05&to_date=2026-11-04", "from_date=2026-02-30&to_date=2026-11-05", "from_date=2026-11-05&to_date=2026-11-05&date_basis=unknown", "from_date=2026-11-05&to_date=2026-11-05&date_basis=issue&date_basis=processing"} {
		values, _ := url.ParseQuery(query)
		if _, _, err := captureParams(values); err == nil {
			t.Fatalf("accepted %s", query)
		}
	}
	for _, basis := range []string{"issue", "processing"} {
		_, selected, err := captureParams(url.Values{"from_date": {"2026-11-05"}, "to_date": {"2026-11-05"}, "date_basis": {basis}})
		if err != nil || selected != basis {
			t.Fatalf("valid date basis rejected: %s", basis)
		}
	}
}
func TestStageParameterValidation(t *testing.T) {
	for _, q := range []string{"from_date=2026-02-30", "from_date=2026-11-05&to_date=2026-01-01", "page=0", "page=-1", "page_size=201", "page=99999999999999999999999", "status=posted", "locator=a&locator=b", "from=2026-01-01"} {
		v, _ := url.ParseQuery(q)
		if _, e := filterParams(v); e == nil {
			t.Fatalf("accepted %s", q)
		}
	}
	f, e := filterParams(url.Values{"from_date": {"2026-11-05"}, "to_date": {"2026-11-05"}, "ticket_number": {"12345"}})
	if e != nil {
		t.Fatal(e)
	}
	where, args := f.where()
	if !strings.Contains(where, "r.mir_date>=?") || !strings.Contains(where, "r.mir_date<=?") || strings.Contains(where, "first_travel_date") || strings.Contains(where, "imported_at") || len(args) != 5 {
		t.Fatal("incorrect date basis")
	}
}
func TestMissingStagingDatabaseFailsConsistently(t *testing.T) {
	h := StageHandler{}
	w := httptest.NewRecorder()
	h.records(w, httptest.NewRequest(http.MethodGet, "/api/v1/mir/records", nil).WithContext(context.Background()))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "mir_database_unavailable") {
		t.Fatal(w.Body.String())
	}
}
