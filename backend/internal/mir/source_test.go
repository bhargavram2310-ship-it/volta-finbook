package mir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type deterministicFixtureParser struct{ calls int }

func (p *deterministicFixtureParser) Parse(_ context.Context, data []byte, name string) ([]Record, error) {
	p.calls++
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	return []Record{{ID: hash + ":r1:p1", FileName: name, SHA256: hash, Status: "Extracted", Fields: map[string]Field{"ticketNumber": {Value: "1234567890123", Lines: []int{1}, FieldIDs: []string{"fixture"}}}, RawLines: []RawLine{{Number: 1, Text: string(data)}}, Issues: []string{}}}, nil
}
func request(handler http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func setup(t *testing.T) (*Service, string, *deterministicFixtureParser) {
	t.Helper()
	folder := t.TempDir()
	parser := &deterministicFixtureParser{}
	service := &Service{ConfigFile: filepath.Join(t.TempDir(), "settings", "mir-source.json"), Parser: parser}
	body, _ := json.Marshal(map[string]string{"path": folder})
	if w := request(service.Source, "POST", "/api/v1/mir/source", string(body)); w.Code != 201 {
		_, pathErr := filepath.EvalSymlinks(folder)
		t.Fatalf("MIR folder setup: HTTP %d %s; temporary path %q; path resolution error: %v", w.Code, w.Body.String(), folder, pathErr)
	}
	return service, folder, parser
}
func TestFolderLockSurvivesRestartAndCannotBeReplaced(t *testing.T) {
	service, folder, _ := setup(t)
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
func TestDateRangeIsInclusiveAndOnlyReadsMatchingMIRFiles(t *testing.T) {
	service, folder, parser := setup(t)
	for _, file := range []struct{ name, date string }{{"first.MIR", "2026-10-01"}, {"last.mir", "2026-10-31"}, {"outside.MIR", "2026-11-01"}, {"notes.txt", "2026-10-15"}} {
		path := filepath.Join(folder, file.name)
		if err := os.WriteFile(path, []byte(file.name), 0600); err != nil {
			t.Fatal(err)
		}
		date, _ := time.ParseInLocation("2006-01-02", file.date, time.Local)
		if err := os.Chtimes(path, date, date); err != nil {
			t.Fatal(err)
		}
	}
	w := request(service.Records, "GET", "/api/v1/mir/records?from=2026-10-01&to=2026-10-31", "")
	var body struct {
		Records []Record `json:"records"`
		Matched int      `json:"matchedFiles"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 200 || len(body.Records) != 2 || body.Matched != 2 || parser.calls != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestInvalidDatesAndTraversalNeverInvokeParser(t *testing.T) {
	service, _, parser := setup(t)
	for _, query := range []string{"from=2026-02-30&to=2026-10-01", "from=2026-11-01&to=2026-10-01", "from=&to="} {
		if w := request(service.Records, "GET", "/api/v1/mir/records?"+query, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if w := request(service.Resolve, "GET", "/api/v1/mir/record?file=..%2Foutside.MIR", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if parser.calls != 0 {
		t.Fatal("invalid requests must not parse files")
	}
}
func TestChangedSourceIsBlockedBeforeProceed(t *testing.T) {
	service, folder, parser := setup(t)
	file := filepath.Join(folder, "ticket.MIR")
	if err := os.WriteFile(file, []byte("original fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256([]byte("original fixture"))
	hash := hex.EncodeToString(old[:])
	query := url.Values{"file": {"ticket.MIR"}, "sha256": {hash}, "id": {hash + ":r1:p1"}}
	if w := request(service.Resolve, "GET", "/api/v1/mir/record?"+query.Encode(), ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := os.WriteFile(file, []byte("changed fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := request(service.Resolve, "GET", "/api/v1/mir/record?"+query.Encode(), ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if parser.calls != 1 {
		t.Fatal("changed file must be rejected before parsing")
	}
}
func TestSourceRejectsRemoteAndCrossOriginRequests(t *testing.T) {
	service, _, _ := setup(t)
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

func TestIdenticalSourceCopiesAreFlaggedWithoutCreatingAnySale(t *testing.T) {
	service, folder, _ := setup(t)
	for _, name := range []string{"original.MIR", "copy.MIR"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("identical synthetic source"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w := request(service.Records, "GET", "/api/v1/mir/records?from=2000-01-01&to=2099-12-31", "")
	var body struct {
		Records []Record `json:"records"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Records) != 2 {
		t.Fatal(w.Body.String())
	}
	duplicates := 0
	for _, record := range body.Records {
		if record.Status == "Duplicate" {
			duplicates++
		}
		if len(record.RawLines) != 0 {
			t.Fatal("list should not duplicate raw evidence per row")
		}
	}
	if duplicates != 1 {
		t.Fatal("identical file copies must produce one duplicate warning")
	}
	files, err := os.ReadDir(folder)
	if err != nil || len(files) != 2 {
		t.Fatal("fetch must not create or remove source files")
	}
}
