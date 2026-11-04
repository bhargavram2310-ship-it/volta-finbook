// Package mir reads a locked local MIR folder. It never creates sales or postings.
package mir

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Field struct {
	Value    string   `json:"value"`
	Lines    []int    `json:"lines"`
	FieldIDs []string `json:"fieldIds"`
}
type RawLine struct {
	Number        int    `json:"number"`
	Text          string `json:"text"`
	RawHex        string `json:"rawHex"`
	TerminatorHex string `json:"terminatorHex"`
}
type Record struct {
	ID         string           `json:"id"`
	FileName   string           `json:"fileName"`
	SHA256     string           `json:"sha256"`
	RecordDate string           `json:"recordDate"`
	Status     string           `json:"status"`
	Fields     map[string]Field `json:"fields"`
	RawLines   []RawLine        `json:"rawLines"`
	Issues     []string         `json:"issues"`
}
type Parser interface {
	Parse(context.Context, []byte, string) ([]Record, error)
}
type PythonParser struct{ Executable, ParserRoot, Bridge string }

func (p PythonParser) Parse(ctx context.Context, data []byte, name string) ([]Record, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Executable, p.Bridge, "--parser-root", p.ParserRoot, "--filename", name)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "PYTHONIOENCODING=utf-8")
	cmd.Stdin = bytes.NewReader(data)
	var output cappedBuffer
	cmd.Stdout = &output
	// Parser stderr can contain raw passenger details. Never forward or log it.
	if err := cmd.Run(); err != nil {
		return nil, errors.New("existing MIR parser unavailable or failed; check parser setup")
	}
	var records []Record
	if err := json.Unmarshal(output.Bytes(), &records); err != nil {
		return nil, errors.New("MIR parser returned an invalid result")
	}
	return records, nil
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16<<20 {
		return 0, errors.New("parser output limit exceeded")
	}
	return b.Buffer.Write(p)
}

type Config struct {
	Path         string `json:"path"`
	Locked       bool   `json:"locked"`
	ConfiguredAt string `json:"configuredAt"`
	Event        string `json:"event"`
}
type Service struct {
	ConfigFile string
	Parser     Parser
	mu         sync.Mutex
}

func DefaultService() *Service {
	root := os.Getenv("FINBOOK_ROOT")
	if root == "" {
		root = ".."
	}
	root, _ = filepath.Abs(root)
	parserRoot := os.Getenv("MIR_PARSER_ROOT")
	if parserRoot == "" {
		parserRoot = filepath.Join(filepath.Dir(root), "IDC-MIR-Reader")
	}
	python := os.Getenv("MIR_PYTHON")
	if python == "" {
		python = filepath.Join(parserRoot, ".venv", "Scripts", "python.exe")
	}
	return &Service{ConfigFile: filepath.Join(root, "backend", ".local", "mir-source.json"), Parser: PythonParser{Executable: python, ParserRoot: parserRoot, Bridge: filepath.Join(root, "backend", "integrations", "mir_bridge.py")}}
}

func (s *Service) config() (Config, error) {
	var config Config
	data, err := os.ReadFile(s.ConfigFile)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	if err == nil && (!config.Locked || !filepath.IsAbs(config.Path)) {
		err = errors.New("invalid saved source")
	}
	return config, err
}
func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" || origin == "http://127.0.0.1:8080" || origin == "http://localhost:8080"
}

func (s *Service) Source(w http.ResponseWriter, r *http.Request) {
	if !localRequest(r) {
		failure(w, 403, "local machine access required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, currentErr := s.config()
	if r.Method == http.MethodGet {
		if errors.Is(currentErr, os.ErrNotExist) {
			respond(w, 200, map[string]any{"path": "", "locked": false})
			return
		}
		if currentErr != nil {
			failure(w, 500, "saved MIR source configuration cannot be read")
			return
		}
		respond(w, 200, current)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		failure(w, 405, "method not allowed")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		failure(w, 415, "JSON content type required")
		return
	}
	var input struct {
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		failure(w, 400, "enter a valid folder path")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		failure(w, 400, "invalid request body")
		return
	}
	input.Path = strings.TrimSpace(input.Path)
	if !filepath.IsAbs(input.Path) {
		failure(w, 400, "enter an absolute local folder path")
		return
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(input.Path))
	if err != nil {
		failure(w, 400, "folder is not accessible on the API machine")
		return
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		failure(w, 400, "path must be an accessible folder")
		return
	}
	if currentErr == nil {
		if current.Path == canonical {
			respond(w, 200, current)
		} else {
			failure(w, 409, "MIR folder is locked and cannot be changed in the app")
		}
		return
	}
	if !errors.Is(currentErr, os.ErrNotExist) {
		failure(w, 500, "saved MIR source configuration cannot be read")
		return
	}
	config := Config{Path: canonical, Locked: true, ConfiguredAt: time.Now().UTC().Format(time.RFC3339), Event: "mir_source_locked"}
	if os.MkdirAll(filepath.Dir(s.ConfigFile), 0700) != nil {
		failure(w, 500, "could not save MIR source configuration")
		return
	}
	// Create exclusively: concurrent requests/processes cannot replace the locked path.
	file, err := os.OpenFile(s.ConfigFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		failure(w, 409, "MIR source was already configured or could not be locked")
		return
	}
	writeErr := json.NewEncoder(file).Encode(config)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		failure(w, 500, "source configuration write failed; ask the administrator to inspect configuration")
		return
	}
	respond(w, 201, config)
}

func validDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil && len(value) == 10
}

func (s *Service) readFile(name string) ([]byte, error) {
	config, err := s.config()
	if err != nil {
		return nil, errors.New("configure the MIR folder first")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, "/\\:") || !strings.EqualFold(filepath.Ext(name), ".mir") {
		return nil, errors.New("invalid MIR file name")
	}
	path := filepath.Join(config.Path, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, errors.New("MIR file is unavailable, not a regular file, or exceeds 2 MB")
	}
	// Do not follow symbolic links outside the locked folder.
	actual, err := filepath.EvalSymlinks(path)
	if err != nil || actual != path {
		return nil, errors.New("symbolic MIR file links are not supported")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open MIR file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("MIR file read failed or exceeds 2 MB")
	}
	return data, nil
}

func (s *Service) Records(w http.ResponseWriter, r *http.Request) {
	if !localRequest(r) {
		failure(w, 403, "local machine access required")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "method not allowed")
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if !validDate(from) || !validDate(to) || from > to {
		failure(w, 400, "choose valid From and To dates with From on or before To")
		return
	}
	config, err := s.config()
	if err != nil {
		failure(w, 503, "configure and lock the MIR folder first")
		return
	}
	files, err := os.ReadDir(config.Path)
	if err != nil {
		failure(w, 503, "locked MIR folder is not accessible")
		return
	}
	records := []Record{}
	issues := []string{}
	matched := 0
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".mir") {
			continue
		}
		info, e := file.Info()
		if e != nil {
			issues = append(issues, file.Name()+": cannot read file metadata")
			continue
		}
		date := info.ModTime().In(time.Local).Format("2006-01-02")
		if date < from || date > to {
			continue
		}
		matched++
		if matched > 100 {
			failure(w, 422, "more than 100 MIR files match; narrow the date range")
			return
		}
		data, e := s.readFile(file.Name())
		if e != nil {
			issues = append(issues, file.Name()+": "+e.Error())
			continue
		}
		parsed, e := s.Parser.Parse(ctx, data, file.Name())
		if e != nil {
			failure(w, 503, e.Error())
			return
		}
		hash := sha256.Sum256(data)
		hashText := hex.EncodeToString(hash[:])
		for _, record := range parsed {
			if len(records) >= 500 {
				failure(w, 422, "more than 500 ticket records match; narrow the date range")
				return
			}
			// Raw evidence is reread and returned on Proceed, not repeated in every list row.
			record.RawLines = []RawLine{}
			record.FileName = file.Name()
			record.SHA256 = hashText
			record.RecordDate = date
			key := record.ID
			if seen[key] {
				record.Status = "Duplicate"
				record.Issues = append(record.Issues, "Identical source record already appears in this result.")
			}
			seen[key] = true
			records = append(records, record)
		}
	}
	respond(w, 200, map[string]any{"records": records, "issues": issues, "dateBasis": "File modified date on the API machine (inclusive)", "matchedFiles": matched})
}

func (s *Service) Resolve(w http.ResponseWriter, r *http.Request) {
	if !localRequest(r) {
		failure(w, 403, "local machine access required")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "method not allowed")
		return
	}
	data, err := s.readFile(r.URL.Query().Get("file"))
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	hash := sha256.Sum256(data)
	hashText := hex.EncodeToString(hash[:])
	if r.URL.Query().Get("sha256") != hashText {
		failure(w, 409, "MIR file changed since fetch; fetch the records again")
		return
	}
	records, err := s.Parser.Parse(r.Context(), data, r.URL.Query().Get("file"))
	if err != nil {
		failure(w, 503, err.Error())
		return
	}
	for _, record := range records {
		if record.ID == r.URL.Query().Get("id") {
			if record.Status == "Failed" || record.Status == "Duplicate" {
				failure(w, 409, "this record cannot proceed")
				return
			}
			record.FileName = r.URL.Query().Get("file")
			record.SHA256 = hashText
			respond(w, 200, record)
			return
		}
	}
	failure(w, 404, "MIR ticket record no longer exists")
}
