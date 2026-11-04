package httpapi

import (
	"encoding/json"
	"net/http"
	"os"

	mysql "github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"
	"voltafinbook/backend/internal/employees"
	"voltafinbook/backend/internal/mir"
)

// The API exposes database readiness and authentication. Financial writes remain
// disabled until their MySQL schema and posting invariants are implemented.
func NewHandler(checkers ...*database.Checker) http.Handler {
	mux := http.NewServeMux()
	var checker *database.Checker
	if len(checkers) > 0 {
		checker = checkers[0]
	}
	var mysqlConfig *mysql.Config
	if checker != nil {
		mysqlConfig = checker.Config
	}
	authHandler := auth.NewHandler(mysqlConfig, os.Getenv("AUTH_COOKIE_SECURE") == "true")
	mux.Handle("/api/v1/auth/", authHandler)
	employeeHandler := employees.NewHandler(mysqlConfig, os.Getenv("AUTH_COOKIE_SECURE") == "true")
	mux.Handle("/api/v1/employees", employeeHandler)
	mux.Handle("/api/v1/employees/", employeeHandler)
	for _, path := range []string{"/api/v1/users", "/api/v1/users/", "/api/v1/roles", "/api/v1/roles/"} {
		mux.Handle(path, employeeHandler)
	}
	mirSource := mir.DefaultService()
	mux.HandleFunc("/api/v1/mir/source", mirSource.Source)
	mux.HandleFunc("/api/v1/mir/records", mirSource.Records)
	mux.HandleFunc("/api/v1/mir/record", mirSource.Resolve)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "volta-finbook", "phase": 2})
	})
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}
		dbStatus := checker.Check(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"phase": 2, "entry_mode": "manual_and_mir_preview", "mir": "local_folder",
			"database": dbStatus.State, "schema_ready": dbStatus.SchemaReady, "migrations": dbStatus.Migrations, "authentication": authenticationState(mysqlConfig != nil),
			"posting": "not_implemented", "tax_rules": "to_be_verified_by_finance",
		})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}
		state := checker.Check(r.Context())
		status := http.StatusServiceUnavailable
		if state.SchemaReady {
			status = http.StatusOK
		}
		writeJSON(w, status, state)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	})
}

func authenticationState(configured bool) string {
	if configured {
		return "implemented"
	}
	return "unavailable"
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", "GET")
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
