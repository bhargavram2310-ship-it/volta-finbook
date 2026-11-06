package reconciliation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/go-sql-driver/mysql"
)

type Statement struct {
	ID         string `json:"id"`
	FileName   string `json:"fileName"`
	Kind       string `json:"kind"`
	RowCount   int    `json:"rowCount"`
	UploadedBy string `json:"uploadedBy"`
	CreatedAt  string `json:"createdAt"`
}
type Repository struct{ DB *sql.DB }

const summaryColumns = "id,file_name,statement_kind,row_count,uploaded_by,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ')"

type scanner interface{ Scan(...any) error }

func scanStatement(row scanner) (Statement, error) {
	var s Statement
	err := row.Scan(&s.ID, &s.FileName, &s.Kind, &s.RowCount, &s.UploadedBy, &s.CreatedAt)
	return s, err
}
func (repo Repository) List(ctx context.Context) ([]Statement, error) {
	rows, err := repo.DB.QueryContext(ctx, "SELECT "+summaryColumns+" FROM reconciliation_statements ORDER BY id DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Statement{}
	for rows.Next() {
		s, e := scanStatement(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
func (repo Repository) Detail(ctx context.Context, id string, offset int) (Statement, []Row, error) {
	s, err := scanStatement(repo.DB.QueryRowContext(ctx, "SELECT "+summaryColumns+" FROM reconciliation_statements WHERE id=?", id))
	if err != nil {
		return s, nil, err
	}
	rows, err := repo.DB.QueryContext(ctx, "SELECT line_number,source_section,source_row,cells FROM reconciliation_statement_rows WHERE statement_id=? ORDER BY line_number LIMIT 100 OFFSET ?", id, offset)
	if err != nil {
		return s, nil, err
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var row Row
		var cells []byte
		if err = rows.Scan(&row.Number, &row.Section, &row.SourceRow, &cells); err != nil {
			return s, nil, err
		}
		if err = json.Unmarshal(cells, &row.Cells); err != nil {
			return s, nil, err
		}
		result = append(result, row)
	}
	return s, result, rows.Err()
}
func (repo Repository) Save(ctx context.Context, name, kind, actor string, source []byte, rows []Row) (Statement, bool, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(source))
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		return Statement{}, false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT INTO reconciliation_statements(file_name,file_sha256,statement_kind,source_bytes,row_count,uploaded_by) VALUES(?,?,?,?,?,?)", name, hash, kind, source, len(rows), actor)
	if err != nil {
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			_ = tx.Rollback()
			s, e := scanStatement(repo.DB.QueryRowContext(ctx, "SELECT "+summaryColumns+" FROM reconciliation_statements WHERE file_sha256=?", hash))
			return s, true, e
		}
		return Statement{}, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Statement{}, false, err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO reconciliation_statement_rows(statement_id,line_number,source_section,source_row,cells) VALUES(?,?,?,?,?)")
	if err != nil {
		return Statement{}, false, err
	}
	defer insert.Close()
	for _, row := range rows {
		cells, e := json.Marshal(row.Cells)
		if e != nil {
			return Statement{}, false, e
		}
		if _, err = insert.ExecContext(ctx, id, row.Number, row.Section, row.SourceRow, cells); err != nil {
			return Statement{}, false, err
		}
	}
	s, err := scanStatement(tx.QueryRowContext(ctx, "SELECT "+summaryColumns+" FROM reconciliation_statements WHERE id=?", strconv.FormatInt(id, 10)))
	if err != nil {
		return Statement{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Statement{}, false, err
	}
	return s, false, nil
}
