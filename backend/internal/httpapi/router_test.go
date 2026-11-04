package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFoundationDoesNotEnableFinancialWrites(t *testing.T) {
	handler := NewHandler()
	for _, path := range []string{"/api/v1/sales", "/api/v1/postings", "/api/v1/login"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", path, w.Code)
		}
	}
}

func TestRoleEndpointsAreMountedBehindAuthentication(t *testing.T) {
	handler := NewHandler()
	for _, testCase := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/roles"},
		{http.MethodPatch, "/api/v1/roles/00000000-0000-4000-8000-000000000001"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s returned %d, want unauthorized", testCase.method, testCase.path, response.Code)
		}
	}
}

func TestStatusMakesUnavailableCapabilitiesExplicit(t *testing.T) {
	w := httptest.NewRecorder()
	NewHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body["database"] != "not_configured" || body["authentication"] != "unavailable" {
		t.Fatal(w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("status must not be cached")
	}
}

func TestHealthRejectsMutation(t *testing.T) {
	w := httptest.NewRecorder()
	NewHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
		t.Fatal(w.Code)
	}
}

func TestReadinessCannotReportSuccessWithoutDatabaseSchema(t *testing.T) {
	w := httptest.NewRecorder()
	NewHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("unconfigured database must not be ready")
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["schemaReady"] != false {
		t.Fatal("readiness state invalid")
	}
}
