// Read-only schema inspection for Sales Workspace compatibility checks.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"voltafinbook/backend/internal/database"
)

func main() {
	cfg, err := database.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT table_name,GROUP_CONCAT(CONCAT(column_name,':',column_type) ORDER BY ordinal_position SEPARATOR ', ') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ('tickets','sales_documents','sales_lines','permissions','company') GROUP BY table_name`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Schema inspection failed")
		os.Exit(1)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var table, columns string
		if err = rows.Scan(&table, &columns); err != nil {
			os.Exit(1)
		}
		result[table] = columns
	}
	if rows.Err() != nil {
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
