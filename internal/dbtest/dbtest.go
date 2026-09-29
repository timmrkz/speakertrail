// Package dbtest gives each test its own migrated PostgreSQL database.
//
// It connects with TEST_DATABASE_URL, whose role must be allowed to create
// databases. Without it the tests are skipped, except in CI, where they fail.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/db"
)

// New creates an empty database, applies all migrations and drops the
// database when the test ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := NewEmpty(t)
	if err := db.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// NewEmpty creates a database without migrations and drops it when the test
// ends.
func NewEmpty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL is not set")
		}
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close(ctx)

	b := make([]byte, 6)
	rand.Read(b)
	name := "speakertrail_test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		admin, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Errorf("connect for cleanup: %v", err)
			return
		}
		defer admin.Close(ctx)
		// Postgres' own autovacuum may still be working on a database that
		// saw a lot of changes, and a normal user cannot end it. It finishes
		// within moments, so the drop is tried again for a few seconds.
		var err2 error
		for range 50 {
			if _, err2 = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err2 == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("drop database: %v", err2)
	})
	return pool
}
