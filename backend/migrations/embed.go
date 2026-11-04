// Package migrations contains legacy PostgreSQL migrations; the MySQL runtime does not execute them.
package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type Migration struct {
	Version             int
	Name, SQL, Checksum string
}

func All() ([]Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	result := []Migration{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		version, parseErr := strconv.Atoi(prefix)
		if !ok || parseErr != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration filename")
		}
		data, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		result = append(result, Migration{Version: version, Name: entry.Name(), SQL: string(data), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for i, migration := range result {
		if migration.Version != i+1 {
			return nil, fmt.Errorf("migration versions must be consecutive and unique")
		}
	}
	return result, nil
}
