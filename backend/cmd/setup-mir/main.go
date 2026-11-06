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
	mode := flag.String("mode", "plan", "plan or apply additive MIR staging tables to an existing installation")
	flag.Parse()
	if *mode != "plan" && *mode != "apply" {
		fmt.Fprintln(os.Stderr, "Use -mode plan or -mode apply")
		os.Exit(1)
	}
	config, err := database.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Database configuration unavailable")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Connect(ctx, config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Database connection unavailable")
		os.Exit(1)
	}
	defer db.Close()
	var status database.MIRSchemaStatus
	if *mode == "apply" {
		status, err = database.ApplyMIRSchema(ctx, db)
	} else {
		status, err = database.MIRSchemaPlan(ctx, db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MIR schema setup failed:", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(status)
}
