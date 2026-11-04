// dbcheck authenticates and inspects catalog metadata in a read-only transaction.
// It never runs migrations or reads business table contents.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type tableInfo struct {
	Schema  string `json:"schema"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Columns int    `json:"columnCount"`
}
type report struct {
	Authenticated      bool        `json:"authenticated"`
	ReadOnly           bool        `json:"readOnly"`
	ServerVersion      string      `json:"serverVersion"`
	CheckedAt          string      `json:"checkedAt"`
	Schemas            []string    `json:"schemas"`
	Tables             []tableInfo `json:"tables"`
	TableListTruncated bool        `json:"tableListTruncated"`
}

func main() {
	credentials := flag.String("credentials", `D:\Projects\DB\postgresql.txt`, "Connection file path (not the connection string)")
	reportPath := flag.String("report", "", "Local metadata report output")
	flag.Parse()
	if *reportPath == "" {
		fmt.Fprintln(os.Stderr, "A report file path is required.")
		os.Exit(1)
	}
	stage := "reading connection configuration"
	if err := inspect(*credentials, *reportPath, &stage); err != nil {
		var serverError *pgconn.PgError
		if errors.As(err, &serverError) {
			fmt.Fprintf(os.Stderr, "Database check failed while %s. PostgreSQL SQLSTATE: %s.\n", stage, serverError.Code)
		} else {
			// Never print raw driver errors: these can contain credentials or hosts.
			fmt.Fprintf(os.Stderr, "Database check failed while %s. Check file access, network, TLS and credentials.\n", stage)
		}
		os.Exit(1)
	}
}

func inspect(credentialFile, reportFile string, stage *string) error {
	data, err := os.ReadFile(credentialFile)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(string(data))
	if strings.HasPrefix(value, "DATABASE_URL") {
		key, rest, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) != "DATABASE_URL" {
			return errors.New("invalid connection setting")
		}
		value = strings.TrimSpace(rest)
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	uri, err := url.Parse(value)
	if err != nil {
		return errors.New("invalid connection URL")
	}
	if uri.Scheme != "postgres" && uri.Scheme != "postgresql" {
		return errors.New("invalid connection scheme")
	}
	query := uri.Query()
	query.Set("sslmode", "verify-full")
	uri.RawQuery = query.Encode()
	config, err := pgx.ParseConfig(uri.String())
	if err != nil {
		return errors.New("invalid database configuration")
	}
	config.ConnectTimeout = 10 * time.Second
	config.Fallbacks = nil
	if config.TLSConfig == nil {
		return errors.New("verified TLS required")
	}
	config.TLSConfig.MinVersion = tls.VersionTLS12
	config.TLSConfig.InsecureSkipVerify = false
	config.Tracer = nil
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	*stage = "authenticating with verified TLS"
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, release := context.WithTimeout(context.Background(), 3*time.Second)
		defer release()
		_ = conn.Close(closeCtx)
	}()
	*stage = "starting a read-only metadata transaction"
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		rollbackCtx, release := context.WithTimeout(context.Background(), 3*time.Second)
		defer release()
		_ = tx.Rollback(rollbackCtx)
	}()
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '10s'"); err != nil {
		return err
	}
	result := report{Authenticated: true, CheckedAt: time.Now().UTC().Format(time.RFC3339), Schemas: []string{}, Tables: []tableInfo{}}
	var readOnly string
	if err = tx.QueryRow(ctx, "SELECT current_setting('transaction_read_only'), current_setting('server_version')").Scan(&readOnly, &result.ServerVersion); err != nil {
		return err
	}
	if readOnly != "on" {
		return errors.New("read-only transaction was not enabled")
	}
	result.ReadOnly = true
	*stage = "reading schema metadata"
	rows, err := tx.Query(ctx, `SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT LIKE 'pg\_%' ESCAPE '\' AND schema_name <> 'information_schema' ORDER BY schema_name`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		result.Schemas = append(result.Schemas, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT t.table_schema,t.table_name,t.table_type,(SELECT count(*) FROM information_schema.columns c WHERE c.table_schema=t.table_schema AND c.table_name=t.table_name) FROM information_schema.tables t WHERE t.table_schema NOT LIKE 'pg\_%' ESCAPE '\' AND t.table_schema <> 'information_schema' ORDER BY t.table_schema,t.table_name LIMIT 2001`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var table tableInfo
		if err = rows.Scan(&table.Schema, &table.Name, &table.Type, &table.Columns); err != nil {
			rows.Close()
			return err
		}
		if len(result.Tables) < 2000 {
			result.Tables = append(result.Tables, table)
		} else {
			result.TableListTruncated = true
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	// End with rollback even though only catalog SELECTs and SET LOCAL were used.
	if err = tx.Rollback(ctx); err != nil {
		return err
	}
	*stage = "writing the local metadata report"
	if err = os.MkdirAll(filepath.Dir(reportFile), 0700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(reportFile, append(encoded, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println("Database password authentication succeeded.")
	fmt.Println("TLS certificate verification succeeded.")
	fmt.Println("Read-only transaction confirmed; no database objects or business records changed.")
	fmt.Printf("Visible schemas: %d. Visible tables/views reported: %d.\n", len(result.Schemas), len(result.Tables))
	fmt.Println("Schema metadata saved to the local .cache/db-inspection.json report.")
	return nil
}
