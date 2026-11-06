package main

import (
	"testing"

	"voltafinbook/backend/internal/database"
	"voltafinbook/backend/migrations"
)

func TestBuildMigrationPlanListsOnlyPending(t *testing.T) {
	all := []migrations.Migration{
		{Version: 1, Name: "0001_foundation.sql", Checksum: "one"},
		{Version: 4, Name: "0004_employee_access.sql", Checksum: "four"},
		{Version: 5, Name: "0005_user_login_method.sql", Checksum: "five"},
	}
	plan := buildMigrationPlan(database.MigrationStatus{Applied: 1, Pending: 2}, all)
	if plan.Status.Applied != 1 || plan.Status.Pending != 2 || len(plan.Pending) != 2 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Pending[0].Version != 4 || plan.Pending[0].Checksum != "four" ||
		plan.Pending[1].Version != 5 || plan.Pending[1].Checksum != "five" {
		t.Fatalf("plan contains wrong pending migrations: %#v", plan.Pending)
	}
}

func TestBuildMigrationPlanHasNoPendingMigrationsWhenCurrent(t *testing.T) {
	plan := buildMigrationPlan(database.MigrationStatus{Applied: 1, Current: true}, []migrations.Migration{
		{Version: 1, Name: "0001_foundation.sql", Checksum: "one"},
	})
	if plan.Pending == nil || len(plan.Pending) != 0 {
		t.Fatalf("expected empty pending list, got %#v", plan.Pending)
	}
}
