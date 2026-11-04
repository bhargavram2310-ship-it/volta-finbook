package migrations

import "testing"

func TestMySQLMigrationsAreOrderedAndChecksummed(t *testing.T) {
	result, err := MySQLAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[0].Version != 1 || result[1].Version != 4 || result[2].Version != 5 {
		t.Fatalf("unexpected MySQL migration sequence: %#v", result)
	}
	for _, migration := range result {
		if migration.SQL == "" || len(migration.Checksum) != 64 {
			t.Fatalf("migration %q is missing SQL or SHA-256 checksum", migration.Name)
		}
	}
}
