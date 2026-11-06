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
	"voltafinbook/backend/migrations"
)

type migrationPlan struct {
	Status  database.MigrationStatus `json:"status"`
	Pending []migrationSummary       `json:"pending"`
}

type migrationSummary struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
}

func buildMigrationPlan(status database.MigrationStatus, all []migrations.Migration) migrationPlan {
	plan := migrationPlan{Status: status, Pending: make([]migrationSummary, 0, status.Pending)}
	for _, migration := range all[status.Applied:] {
		plan.Pending = append(plan.Pending, migrationSummary{
			Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum,
		})
	}
	return plan
}

func main() {
	mode := flag.String("mode", "status", "plan, status or apply")
	flag.Parse()
	if err := run(*mode); err != nil {
		fmt.Fprintln(os.Stderr, "Migration command failed:", err)
		os.Exit(1)
	}
}
func run(mode string) error {
	if mode != "plan" && mode != "status" && mode != "apply" {
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
	if mode == "plan" {
		all, err := migrations.MySQLAll()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(buildMigrationPlan(status, all))
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}
