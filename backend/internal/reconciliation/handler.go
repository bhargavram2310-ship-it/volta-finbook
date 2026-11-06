package reconciliation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"
)

type Handler struct{ Config *mysql.Config }

func NewHandler(config *mysql.Config) http.Handler {
	h := &Handler{config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/reconciliation/statements", h.list)
	mux.HandleFunc("POST /api/v1/reconciliation/statements", h.upload)
	mux.HandleFunc("GET /api/v1/reconciliation/statements/{id}", h.detail)
	return auth.RequireAuthenticated(config, mux)
}
func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func (h *Handler) open(w http.ResponseWriter, r *http.Request) (Repository, context.Context, func(), bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	db, err := database.Connect(ctx, h.Config)
	if err != nil {
		cancel()
		failure(w, 503, "Statement service unavailable. Check the database connection.")
		return Repository{}, ctx, func() {}, false
	}
	return Repository{db}, ctx, func() { db.Close(); cancel() }, true
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	repo, ctx, close, ok := h.open(w, r)
	if !ok {
		return
	}
	defer close()
	statements, err := repo.List(ctx)
	if err != nil {
		failure(w, 503, "Statements could not be loaded. Check the reconciliation database setup.")
		return
	}
	respond(w, 200, map[string]any{"statements": statements})
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		failure(w, 400, "Invalid statement ID.")
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > maxRows {
			failure(w, 400, "Invalid row offset.")
			return
		}
	}
	repo, ctx, close, ok := h.open(w, r)
	if !ok {
		return
	}
	defer close()
	s, rows, err := repo.Detail(ctx, id, offset)
	if errors.Is(err, sql.ErrNoRows) {
		failure(w, 404, "Statement not found.")
		return
	}
	if err != nil {
		failure(w, 503, "Saved statement rows could not be read.")
		return
	}
	respond(w, 200, map[string]any{"statement": s, "rows": rows, "offset": offset})
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		failure(w, 403, "Invalid request origin.")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			failure(w, 403, "Invalid request origin.")
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		failure(w, 400, "Upload one statement up to 10 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["file"]
	if len(files) != 1 || len(r.MultipartForm.File) != 1 {
		failure(w, 400, "Upload exactly one statement.")
		return
	}
	kind := r.FormValue("kind")
	if kind != "bank" && kind != "supplier" {
		failure(w, 400, "Select Bank or Supplier as the statement type.")
		return
	}
	name := filepath.Base(strings.ReplaceAll(files[0].Filename, "\\", "/"))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 {
		failure(w, 400, "Statement filename is too long or invalid.")
		return
	}
	if files[0].Size <= 0 || files[0].Size > MaxFileSize {
		failure(w, 400, "Choose a nonempty statement up to 10 MB.")
		return
	}
	file, err := files[0].Open()
	if err != nil {
		failure(w, 400, "Statement could not be opened.")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		failure(w, 400, "Statement could not be read.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	rows, err := Extract(ctx, name, raw)
	if err != nil {
		failure(w, 422, err.Error())
		return
	}
	repo, dbCtx, close, ok := h.open(w, r)
	if !ok {
		return
	}
	defer close()
	actor, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		failure(w, 401, "Authentication required.")
		return
	}
	s, duplicate, err := repo.Save(dbCtx, name, kind, actor.ID, raw, rows)
	if err != nil {
		failure(w, 503, "The import could not be confirmed. Check saved statements and the database setup, then retry. Duplicate files will not be imported twice.")
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	respond(w, status, map[string]any{"statement": s, "duplicate": duplicate})
}
