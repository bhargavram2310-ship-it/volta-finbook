package mir

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func request(handler http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func setup(t *testing.T) (*Service, string) {
	t.Helper()
	folder := t.TempDir()
	service := &Service{ConfigFile: filepath.Join(t.TempDir(), "settings", "mir-source.json")}
	body, _ := json.Marshal(map[string]string{"path": folder})
	if w := request(service.Source, "POST", "/api/v1/mir/source", string(body)); w.Code != 201 {
		_, pathErr := filepath.EvalSymlinks(folder)
		t.Fatalf("MIR folder setup: HTTP %d %s; temporary path %q; path resolution error: %v", w.Code, w.Body.String(), folder, pathErr)
	}
	return service, folder
}
func TestFolderLockSurvivesRestartAndCannotBeReplaced(t *testing.T) {
	service, folder := setup(t)
	// Source deliberately saves the canonical path. Windows TEMP may use an
	// 8.3 alias (for example BARGHA~1) that expands to a different path string.
	canonicalFolder, err := filepath.EvalSymlinks(folder)
	if err != nil {
		t.Fatalf("cannot resolve the temporary MIR folder: %v", err)
	}
	restarted := &Service{ConfigFile: service.ConfigFile}
	w := request(restarted.Source, "GET", "/api/v1/mir/source", "")
	var saved Config
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &saved) != nil || !saved.Locked || saved.ConfiguredAt == "" {
		t.Fatal(w.Body.String())
	}
	if saved.Path != canonicalFolder {
		t.Fatalf("saved folder %q does not match canonical source folder %q (original path %q)", saved.Path, canonicalFolder, folder)
	}
	body, _ := json.Marshal(map[string]string{"path": t.TempDir()})
	if w := request(restarted.Source, "POST", "/api/v1/mir/source", string(body)); w.Code != 409 {
		t.Fatal(w.Code)
	}
	body, _ = json.Marshal(map[string]string{"path": folder})
	if w := request(restarted.Source, "POST", "/api/v1/mir/source", string(body)); w.Code != 200 {
		t.Fatal("same folder setup should be idempotent", w.Code)
	}
}
func TestSourceRejectsRemoteAndCrossOriginRequests(t *testing.T) {
	service, _ := setup(t)
	for _, remote := range []string{"192.0.2.1:5000", "127.0.0.1:5000"} {
		r := httptest.NewRequest("GET", "/api/v1/mir/source", nil)
		r.RemoteAddr = remote
		if strings.HasPrefix(remote, "127.") {
			r.Header.Set("Origin", "https://untrusted.example")
		}
		w := httptest.NewRecorder()
		service.Source(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}
