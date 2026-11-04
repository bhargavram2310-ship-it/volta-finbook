package database

import (
	"context"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

type Status struct {
	State       string           `json:"state"`
	SchemaReady bool             `json:"schemaReady"`
	Migrations  *MigrationStatus `json:"migrations,omitempty"`
}
type Checker struct {
	Config *mysql.Config
	slots  chan struct{}
}

func NewChecker(config *mysql.Config) *Checker {
	return &Checker{Config: config, slots: make(chan struct{}, 2)}
}
func (checker *Checker) Check(ctx context.Context) Status {
	if checker == nil || checker.Config == nil {
		return Status{State: "not_configured"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case checker.slots <- struct{}{}:
		defer func() { <-checker.slots }()
	case <-ctx.Done():
		return Status{State: "unavailable"}
	}
	conn, err := Connect(ctx, checker.Config)
	if err != nil {
		return Status{State: "unavailable"}
	}
	defer func() { _ = conn.Close() }()
	history, err := CheckMigrations(ctx, conn)
	if err != nil {
		return Status{State: "schema_incompatible"}
	}
	state := "connected_schema_pending"
	if history.Current {
		state = "connected"
	}
	return Status{State: state, SchemaReady: history.Current, Migrations: &history}
}
