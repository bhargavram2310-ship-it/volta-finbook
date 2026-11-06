package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
	"voltafinbook/backend/internal/database"
)

func main() {
	mode := flag.String("mode", "plan", "plan or apply reconciliation statement table setup")
	flag.Parse()
	if *mode != "plan" && *mode != "apply" {
		fmt.Fprintln(os.Stderr, "Use -mode plan or -mode apply")
		os.Exit(1)
	}
	if err := run(*mode == "apply"); err != nil {
		fmt.Fprintln(os.Stderr, "Reconciliation setup failed:", err)
		os.Exit(1)
	}
}
func run(apply bool) error {
	cfg, err := database.LoadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ready, err := database.SetupReconciliation(ctx, db, apply)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": ready, "tables": []string{"reconciliation_statements", "reconciliation_statement_rows"}})
}
