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
	mode := flag.String("mode", "plan", "plan or apply sales type master setup")
	flag.Parse()
	if *mode != "plan" && *mode != "apply" {
		fmt.Fprintln(os.Stderr, "Use -mode plan or -mode apply")
		os.Exit(1)
	}
	cfg, err := database.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Database configuration unavailable")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Database connection unavailable")
		os.Exit(1)
	}
	defer db.Close()
	ready, err := database.SetupSalesTypes(ctx, db, *mode == "apply")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Sales type setup failed:", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": ready, "master": "sales_types", "defaults": []string{"International", "Domestic", "Miscellaneous"}})
}
