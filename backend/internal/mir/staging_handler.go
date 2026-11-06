package mir

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"
	"voltafinbook/backend/internal/mir/decoder"
)

type StageHandler struct {
	Config *mysql.Config
	Source *Service
}

func NewStageHandler(config *mysql.Config, sources ...*Service) http.Handler {
	h := &StageHandler{Config: config, Source: DefaultService()}
	if len(sources) > 0 {
		h.Source = sources[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/mir/import", h.importFiles)
	mux.HandleFunc("POST /api/v1/mir/pull", h.pullFiles)
	mux.HandleFunc("GET /api/v1/mir/capture", h.capture)
	mux.HandleFunc("GET /api/v1/mir/capture/settings", h.captureSettings)
	mux.HandleFunc("GET /api/v1/mir/capture/{id}/details", h.captureDetails)
	mux.HandleFunc("GET /api/v1/mir/capture/{id}/sale", h.captureSale)
	mux.HandleFunc("GET /api/v1/mir/capture/{id}/draft", h.captureDraft)
	mux.HandleFunc("GET /api/v1/mir/records", h.records)
	mux.HandleFunc("GET /api/v1/mir/records/{id}", h.detail)
	mux.HandleFunc("POST /api/v1/mir/records/{id}/reprocess", h.reprocess)
	mux.HandleFunc("GET /api/v1/mir/batches", h.batches)
	mux.HandleFunc("GET /api/v1/mir/batches/{id}", h.batches)
	for path, method := range map[string]string{"/api/v1/mir/pull": "POST", "/api/v1/mir/capture": "GET", "/api/v1/mir/import": "POST", "/api/v1/mir/records": "GET", "/api/v1/mir/records/{id}": "GET", "/api/v1/mir/records/{id}/reprocess": "POST", "/api/v1/mir/batches": "GET", "/api/v1/mir/batches/{id}": "GET"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", method)
			failure(w, http.StatusMethodNotAllowed, "method_not_allowed")
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { failure(w, http.StatusNotFound, "not_found") })
	return auth.RequireAuthenticated(config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				failure(w, 403, "invalid_origin")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host {
					failure(w, 403, "invalid_origin")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	}))
}
func (h *StageHandler) repository(w http.ResponseWriter, r *http.Request) (Repository, context.Context, context.CancelFunc, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	db, e := database.Connect(ctx, h.Config)
	if e != nil {
		cancel()
		failure(w, 503, "mir_database_unavailable")
		return Repository{}, ctx, func() {}, false
	}
	return Repository{db}, ctx, func() { db.Close(); cancel() }, true
}

func (h *StageHandler) importFiles(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if e := r.ParseMultipartForm(4 << 20); e != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		failure(w, 400, "invalid_multipart_or_request_exceeds_32_mib")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > 100 {
		failure(w, 400, "upload_1_to_100_files_using_files_field")
		return
	}
	sources := make([]importSource, 0, len(files))
	for _, f := range files {
		sources = append(sources, importSource{Name: f.Filename, Size: f.Size, Open: func() (io.ReadCloser, error) { return f.Open() }})
	}
	h.importSources(w, r, sources)
}

type importSource struct {
	Name string
	Size int64
	Open func() (io.ReadCloser, error)
}

func (h *StageHandler) pullFiles(w http.ResponseWriter, r *http.Request) {
	if h.Source == nil {
		failure(w, 503, "configure_the_mir_source_folder_first")
		return
	}
	config, err := h.Source.config()
	if err != nil {
		failure(w, 503, "configure_and_lock_the_mir_source_folder_first")
		return
	}
	entries, err := os.ReadDir(config.Path)
	if err != nil {
		failure(w, 503, "mir_source_folder_unavailable")
		return
	}
	sources := []importSource{}
	var total int64
	for _, entry := range entries {
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".mir") {
			continue
		}
		path := filepath.Join(config.Path, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		total += info.Size()
		if len(sources) >= 100 || total > 30<<20 {
			failure(w, 422, "folder_exceeds_100_files_or_30_mib_use_upload_batches")
			return
		}
		sources = append(sources, importSource{Name: entry.Name(), Size: info.Size(), Open: func() (io.ReadCloser, error) {
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			current, err := file.Stat()
			if err != nil || !os.SameFile(info, current) {
				file.Close()
				return nil, errors.New("source_file_changed")
			}
			return file, nil
		}})
	}
	if len(sources) == 0 {
		failure(w, 400, "no_mir_files_in_the_configured_folder")
		return
	}
	h.importSources(w, r, sources)
}

func (h *StageHandler) importSources(w http.ResponseWriter, r *http.Request, files []importSource) {
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	actor, _ := auth.PrincipalFromContext(ctx)
	actorBytes, e := hex.DecodeString(strings.ReplaceAll(actor.ID, "-", ""))
	if e != nil || len(actorBytes) != 16 {
		failure(w, 401, "authentication_required")
		return
	}
	id, e := repo.StartBatch(ctx, actorBytes)
	if e != nil {
		failure(w, 503, batchCreateError(e))
		return
	}
	b := Batch{ID: id, Errors: []ImportError{}}
	// A detached finalizer makes a cancelled request visible as interrupted in
	// batch history, with already committed records retained and accounted for.
	defer func() {
		if b.Status == "" {
			b.Status = "interrupted"
		}
		finalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = repo.FinishBatch(finalCtx, b)
	}()
	for _, f := range files {
		name := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		fail := func(n int, msg string) { b.Failed++; b.Errors = append(b.Errors, ImportError{name, n, msg}) }
		if len(name) > 255 || !strings.EqualFold(filepath.Ext(name), ".mir") || f.Size == 0 || f.Size > 2<<20 {
			fail(0, "invalid_mir_filename_or_file_size_limit_2_mib")
			continue
		}
		file, e := f.Open()
		if e != nil {
			fail(0, "file_read_failed")
			continue
		}
		data, e := io.ReadAll(io.LimitReader(file, (2<<20)+1))
		file.Close()
		if e != nil || len(data) > 2<<20 {
			fail(0, "file_read_failed_or_too_large")
			continue
		}
		// Content validation precedes persistence. Invalid text is a file error;
		// recognizable but malformed MIR is retained for inspection and reprocessing.
		if len(data) < 2 || string(data[:2]) != "T5" {
			fail(0, "unsupported_content_expected_mir_header_T5")
			continue
		}
		source, duplicate, e := repo.Source(ctx, id, name, data)
		if e != nil {
			fail(0, "source_storage_failed")
			continue
		}
		records, e := decoder.ParseStream(data, name)
		if e != nil {
			fail(0, "parser_failed_source_retained")
			if _, saveErr := repo.DB.ExecContext(ctx, `INSERT INTO mir_batch_record_error(batch_id,source_file_id,record_number,error_code) VALUES(?,?,0,?)`, id, source, "parser_failed_source_retained"); saveErr != nil {
				b.Status = "interrupted"
			}
			continue
		}
		for i, parsed := range records {
			if duplicate {
				var count int
				if e := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_record WHERE source_file_id=? AND record_number=?`, source, i+1).Scan(&count); e != nil {
					fail(i+1, "duplicate_lookup_failed")
					continue
				}
				if count > 0 {
					b.Duplicates++
					continue
				}
			}
			rid, e := repo.Stage(ctx, source, i+1, parsed, 0)
			if e != nil {
				var driverError *mysql.MySQLError
				if errors.As(e, &driverError) && driverError.Number == 1062 {
					var count int
					if lookupErr := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mir_record WHERE source_file_id=? AND record_number=?`, source, i+1).Scan(&count); lookupErr == nil && count > 0 {
						b.Duplicates++
						continue
					}
				}
				fail(i+1, "record_transaction_failed_source_retained")
				_, saveErr := repo.DB.ExecContext(ctx, `INSERT INTO mir_batch_record_error(batch_id,source_file_id,record_number,error_code) VALUES(?,?,?,?)`, id, source, i+1, "record_transaction_failed")
				if saveErr != nil {
					b.Status = "interrupted"
				}
				continue
			}
			var status string
			if e := repo.DB.QueryRowContext(ctx, `SELECT status FROM mir_record WHERE id=?`, rid).Scan(&status); e != nil {
				b.Status = "interrupted"
				fail(i+1, "record_status_lookup_failed")
				continue
			}
			if status == "failed" {
				fail(i+1, "validation_failed_record_retained")
			} else {
				b.Imported++
			}
		}
	}
	if b.Status == "" {
		b.Status = "completed"
		if b.Failed > 0 {
			b.Status = "completed_with_errors"
		}
	}
	if e := repo.FinishBatch(ctx, b); e != nil {
		failure(w, 503, "batch_summary_save_failed_records_retained")
		return
	}
	respond(w, 201, b)
}

func batchCreateError(err error) string {
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1146:
			return "MIR staging tables are missing. Run the MIR database setup before extracting files."
		case 1054:
			return "MIR staging tables need a schema update before files can be extracted."
		case 1142:
			return "The database account does not have permission to create MIR import batches."
		case 1452:
			return "The sign-in account changed. Sign in again before extracting files."
		}
	}
	return "MIR import batch could not be created. Check the database connection and try again."
}

func filterParams(q url.Values) (RecordFilter, error) {
	f := RecordFilter{From: q.Get("from_date"), To: q.Get("to_date"), Status: q.Get("status"), Locator: q.Get("locator"), Ticket: q.Get("ticket_number"), Passenger: q.Get("passenger"), Filename: q.Get("filename"), Search: q.Get("search"), Page: 1, PageSize: 50}
	for k, values := range q {
		if len(values) != 1 {
			return f, errors.New("duplicate_parameter")
		}
		switch k {
		case "from_date", "to_date", "status", "locator", "ticket_number", "passenger", "filename", "search", "page", "page_size":
		default:
			return f, errors.New("unknown_parameter")
		}
		if len(values[0]) > 255 {
			return f, errors.New("parameter_too_long")
		}
	}
	if (f.From != "" && !validDate(f.From)) || (f.To != "" && !validDate(f.To)) || (f.From != "" && f.To != "" && f.From > f.To) {
		return f, errors.New("invalid_mir_date_range")
	}
	if f.Status != "" && f.Status != "staged" && f.Status != "review" && f.Status != "failed" {
		return f, errors.New("invalid_import_status")
	}
	for key, target := range map[string]*int{"page": &f.Page, "page_size": &f.PageSize} {
		if q.Get(key) == "" {
			continue
		}
		n, e := strconv.Atoi(q.Get(key))
		if e != nil || n < 1 || n > 1000000 {
			return f, errors.New("invalid_pagination")
		}
		*target = n
	}
	if f.PageSize > 200 {
		return f, errors.New("page_size_exceeds_200")
	}
	return f, nil
}
func (h *StageHandler) records(w http.ResponseWriter, r *http.Request) {
	f, e := filterParams(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	rows, total, e := repo.List(ctx, f)
	if e != nil {
		failure(w, 503, "mir_records_unavailable")
		return
	}
	respond(w, 200, map[string]any{"records": rows, "total": total, "page": f.Page, "page_size": f.PageSize, "date_basis": "T50DTE", "import_mode": "STAGE"})
}
func (h *StageHandler) detail(w http.ResponseWriter, r *http.Request) {
	id, e := parseID(r.PathValue("id"))
	if e != nil {
		failure(w, 400, "invalid_record_id")
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	d, e := repo.Detail(ctx, id)
	if errors.Is(e, sql.ErrNoRows) {
		failure(w, 404, "mir_record_not_found")
		return
	}
	if e != nil {
		failure(w, 503, "mir_record_unavailable")
		return
	}
	respond(w, 200, d)
}
func (h *StageHandler) batches(w http.ResponseWriter, r *http.Request) {
	id := int64(0)
	var e error
	if r.PathValue("id") != "" {
		id, e = parseID(r.PathValue("id"))
		if e != nil {
			failure(w, 400, "invalid_batch_id")
			return
		}
	}
	page := 1
	if value := r.URL.Query().Get("page"); value != "" {
		page, e = strconv.Atoi(value)
		if e != nil || page < 1 || page > 1000000 {
			failure(w, 400, "invalid_page")
			return
		}
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	rows, e := repo.Batches(ctx, id, page)
	if e != nil {
		failure(w, 503, "mir_batches_unavailable")
		return
	}
	if id > 0 {
		if len(rows) == 0 {
			failure(w, 404, "mir_batch_not_found")
			return
		}
		respond(w, 200, rows[0])
		return
	}
	respond(w, 200, map[string]any{"batches": rows, "page": page, "page_size": 50})
}
func (h *StageHandler) reprocess(w http.ResponseWriter, r *http.Request) {
	id, e := parseID(r.PathValue("id"))
	if e != nil {
		failure(w, 400, "invalid_record_id")
		return
	}
	repo, ctx, close, ok := h.repository(w, r)
	if !ok {
		return
	}
	defer close()
	var raw []byte
	var filename string
	var source int64
	var number int
	e = repo.DB.QueryRowContext(ctx, `SELECT r.raw_record,s.filename,r.source_file_id,r.record_number FROM mir_record r JOIN mir_source_file s ON s.id=r.source_file_id WHERE r.id=?`, id).Scan(&raw, &filename, &source, &number)
	if errors.Is(e, sql.ErrNoRows) {
		failure(w, 404, "mir_record_not_found")
		return
	}
	if e != nil {
		failure(w, 503, "mir_record_unavailable")
		return
	}
	records, e := decoder.ParseStream(raw, filename)
	if e != nil || len(records) != 1 {
		failure(w, 422, "reprocessing_cannot_establish_single_record")
		return
	}
	if _, e := repo.Stage(ctx, source, number, records[0], id); e != nil {
		failure(w, 503, "reprocess_transaction_failed_previous_data_retained")
		return
	}
	d, e := repo.Detail(ctx, id)
	if e != nil {
		failure(w, 503, "mir_record_unavailable")
		return
	}
	respond(w, 200, d)
}
