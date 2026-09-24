// Package settings reads the settings table. Every number Tim can tune
// lives there, with its default from the migrations.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Settings is a snapshot of the settings table.
type Settings map[string]json.RawMessage

// Load reads all settings.
func Load(ctx context.Context, pool *pgxpool.Pool) (Settings, error) {
	rows, err := pool.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()
	s := Settings{}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		s[k] = v
	}
	return s, rows.Err()
}

// Int returns a whole number, or def when missing or not a number.
func (s Settings) Int(key string, def int) int {
	var f float64
	if json.Unmarshal(s[key], &f) != nil {
		return def
	}
	return int(f)
}

// Float returns a number, or def.
func (s Settings) Float(key string, def float64) float64 {
	var f float64
	if json.Unmarshal(s[key], &f) != nil {
		return def
	}
	return f
}

// Bool returns a boolean, or def.
func (s Settings) Bool(key string, def bool) bool {
	var b bool
	if json.Unmarshal(s[key], &b) != nil {
		return def
	}
	return b
}

// String returns a string, or def.
func (s Settings) String(key, def string) string {
	var v string
	if json.Unmarshal(s[key], &v) != nil {
		return def
	}
	return v
}

// Strings returns a list of strings, or def.
func (s Settings) Strings(key string, def []string) []string {
	var v []string
	if json.Unmarshal(s[key], &v) != nil {
		return def
	}
	return v
}

// Days returns a number of days as a duration.
func (s Settings) Days(key string, def int) time.Duration {
	return time.Duration(s.Int(key, def)) * 24 * time.Hour
}

// Region returns the region's cities in lower case, for lookups.
func (s Settings) Region() map[string]bool {
	out := map[string]bool{}
	for _, c := range s.Strings("region", nil) {
		out[strings.ToLower(c)] = true
	}
	return out
}
