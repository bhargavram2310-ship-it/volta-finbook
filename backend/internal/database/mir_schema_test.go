package database

import (
	"strings"
	"testing"
)

func TestMIRSetupContainsOnlyNewStagingTableCreates(t *testing.T) {
	statements, names, checksum, err := mirSchemaStatements()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 20 || len(names) != len(statements) || len(checksum) != 64 {
		t.Fatal("incomplete MIR staging setup")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] || !strings.HasPrefix(name, "mir_") || name == "mir_files" || name == "mir_records" || name == "mir_fields" {
			t.Fatalf("unsafe MIR staging target %s", name)
		}
		seen[name] = true
	}
	if !seen["mir_import_batch"] || !seen["mir_passenger"] {
		t.Fatal("MIR batch and passenger tables required")
	}
}
