// Package mir configures a locked source folder and stages MIR uploads with the
// native Go decoder. It never creates sales or postings.
package mir

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Path         string `json:"path"`
	Locked       bool   `json:"locked"`
	ConfiguredAt string `json:"configuredAt"`
	Event        string `json:"event"`
}
type Service struct {
	ConfigFile string
	mu         sync.Mutex
}

func DefaultService() *Service {
	root := os.Getenv("FINBOOK_ROOT")
	if root == "" {
		root = ".."
	}
	root, _ = filepath.Abs(root)
	return &Service{ConfigFile: filepath.Join(root, "backend", ".local", "mir-source.json")}
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
