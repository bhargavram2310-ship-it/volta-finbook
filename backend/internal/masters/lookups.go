package masters

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"voltafinbook/backend/internal/auth"
	"voltafinbook/backend/internal/database"
)

type Option struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// SalesLookups exposes only active masters. Drafts retain IDs for relationships
// and currency codes; source text is never used to guess a master relationship.
func SalesLookups(config *mysql.Config) http.Handler {
	return auth.RequireAuthenticated(config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "method_not_allowed"})
			return
		}
		fail := func() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Master lists could not be loaded. Try again."})
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		db, err := database.Connect(ctx, config)
		if err != nil {
			fail()
			return
		}
		defer db.Close()
		queries := map[string]string{
			"airlines":   `SELECT LOWER(HEX(id)),code,name FROM airlines WHERE active=TRUE AND deleted_at IS NULL ORDER BY name,code`,
			"currencies": `SELECT code,code,name FROM currencies WHERE active=TRUE AND deleted_at IS NULL ORDER BY code`,
			"customers":  `SELECT LOWER(HEX(c.id)),p.code,p.legal_name FROM customers c JOIN parties p ON p.id=c.party_id WHERE p.active=TRUE AND p.deleted_at IS NULL ORDER BY p.legal_name,p.code`,
			"suppliers":  `SELECT LOWER(HEX(s.id)),p.code,p.legal_name FROM suppliers s JOIN parties p ON p.id=s.party_id WHERE p.active=TRUE AND p.deleted_at IS NULL ORDER BY p.legal_name,p.code`,
		}
		result := make(map[string][]Option)
		for key, query := range queries {
			rows, err := db.QueryContext(ctx, query)
			if err != nil {
				fail()
				return
			}
			options := []Option{}
			for rows.Next() {
				var option Option
				if err = rows.Scan(&option.ID, &option.Code, &option.Name); err != nil {
					break
				}
				options = append(options, option)
			}
			rowsErr := rows.Err()
			rows.Close()
			if err != nil || rowsErr != nil {
				fail()
				return
			}
			result[key] = options
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
}
