package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed mysql/*.sql
var mysqlFiles embed.FS

func MySQLAll() ([]Migration, error) {
	entries, err := fs.ReadDir(mysqlFiles, "mysql")
	if err != nil {
		return nil, err
	}
	result := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		version, parseErr := strconv.Atoi(prefix)
		if !ok || parseErr != nil || version <= 0 {
			return nil, fmt.Errorf("invalid MySQL migration filename %q", entry.Name())
		}
		name := path.Join("mysql", entry.Name())
		data, err := mysqlFiles.ReadFile(name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		result = append(result, Migration{
			Version: version, Name: entry.Name(), SQL: string(data),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for i := 1; i < len(result); i++ {
		if result[i].Version == result[i-1].Version {
			return nil, fmt.Errorf("duplicate MySQL migration version %d", result[i].Version)
		}
	}
	return result, nil
}
