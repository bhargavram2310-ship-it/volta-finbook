package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"voltafinbook/backend/internal/database"
)

func TestLoginRejectsInvalidJSONWithoutDatabase(t *testing.T) {
	handler := NewHandler(nil, false)
	for _, body := range []string{
		`{"username":"admin","password":"secret","unexpected":true}`,
		`{"username":"admin","password":"secret"} {}`,
		strings.Repeat("x", maxLoginBody+1),
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("malformed login request was accepted or leaked input: %d", response.Code)
		}
	}
}

func TestLoginRequiresDatabaseAndUsesGenericInvalidCredentials(t *testing.T) {
	handler := NewHandler(nil, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "secret") {
		t.Fatal("unconfigured authentication should fail safely without exposing credentials")
	}
}

func TestAuthMethodAndSessionTokenValidation(t *testing.T) {
	handler := NewHandler(nil, false)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatal("login must reject non-POST requests")
	}

	token, expectedHash, err := newSessionToken()
	if err != nil {
		t.Fatal("could not create session token")
	}
	actualHash, ok := hashSessionToken(token)
	if !ok || actualHash != expectedHash {
		t.Fatal("issued session token did not validate")
	}
	if _, ok := hashSessionToken("not-a-session-token"); ok {
		t.Fatal("invalid session token was accepted")
	}
}

func TestBinaryUUIDRoundTrip(t *testing.T) {
	value, err := newUUIDBytes()
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := formatUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 36 || formatted[14] != '4' || (formatted[19] != '8' && formatted[19] != '9' && formatted[19] != 'a' && formatted[19] != 'b') {
		t.Fatalf("unexpected RFC 4122 UUID format: %q", formatted)
	}
}

func TestFormatUUIDRejectsUnexpectedBinaryLength(t *testing.T) {
	if _, err := formatUUID([]byte(strings.Repeat("x", 15))); err == nil {
		t.Fatal("invalid binary UUID length was accepted")
	}
}

func TestNewPasswordPolicy(t *testing.T) {
	if !validNewPassword("six789") || !validNewPassword("éééééé") {
		t.Fatal("password with six characters should be accepted")
	}
	if validNewPassword("12345") || validNewPassword(strings.Repeat("a", 73)) {
		t.Fatal("password outside the 6-character to 72-byte bounds was accepted")
	}
}

func TestMySQLLoginQueryIntegration(t *testing.T) {
	if os.Getenv("FINBOOK_MYSQL_INTEGRATION") != "1" {
		t.Skip("set FINBOOK_MYSQL_INTEGRATION=1 to run against the configured MySQL database")
	}
	config, err := database.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	token, _, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(config, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(fmt.Sprintf(`{"username":"schema-probe-%s","password":"not-a-password"}`, token)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "invalid_credentials") {
		t.Fatalf("login query was incompatible with the installed MySQL schema: %d %s", response.Code, response.Body.String())
	}
}
