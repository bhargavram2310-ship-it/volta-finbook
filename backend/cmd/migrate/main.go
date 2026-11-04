package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"voltafinbook/backend/internal/database"
)

func main() {
	mode := flag.String("mode", "status", "plan, status or apply")
	flag.Parse()
	if err := run(*mode); err != nil {
		fmt.Fprintln(os.Stderr, "Migration command failed:", err)
		os.Exit(1)
	}
}
func run(mode string) error {
	if mode == "plan" {
		return errors.New("MySQL migrations are not available yet; PostgreSQL migration SQL is not a valid MySQL plan")
	}
	if mode != "status" && mode != "apply" {
		return errors.New("invalid mode")
	}
	config, err := database.LoadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := database.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer conn.Close()
	var status database.MigrationStatus
	if mode == "apply" {
		status, err = database.ApplyMigrations(ctx, conn)
	} else {
		status, err = database.CheckMigrations(ctx, conn)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}
