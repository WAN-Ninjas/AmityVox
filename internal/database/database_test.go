package database

import (
	"io/fs"
	"strings"
	"testing"
)

func TestMigrationsEmbedded(t *testing.T) {
	// Verify that the embedded migrations filesystem contains expected files.
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("reading embedded migrations dir: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no migration files embedded")
	}

	var hasUp, hasDown bool
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			hasUp = true
		}
		if strings.HasSuffix(name, ".down.sql") {
			hasDown = true
		}
	}

	if !hasUp {
		t.Error("no .up.sql migration files found")
	}
	if !hasDown {
		t.Error("no .down.sql migration files found")
	}
}

func TestMigration001_Content(t *testing.T) {
	// Verify the initial migration file is readable and contains expected SQL.
	data, err := migrationsFS.ReadFile("migrations/001_initial_schema.up.sql")
	if err != nil {
		t.Fatalf("reading 001_initial_schema.up.sql: %v", err)
	}

	content := string(data)
	expectedTables := []string{
		"CREATE TABLE instances",
		"CREATE TABLE users",
		"CREATE TABLE guilds",
		"CREATE TABLE channels",
		"CREATE TABLE messages",
		"CREATE TABLE roles",
	}

	for _, table := range expectedTables {
		if !strings.Contains(content, table) {
			t.Errorf("migration missing expected SQL: %s", table)
		}
	}
}

func TestMigration001_Down(t *testing.T) {
	data, err := migrationsFS.ReadFile("migrations/001_initial_schema.down.sql")
	if err != nil {
		t.Fatalf("reading 001_initial_schema.down.sql: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "DROP TABLE") {
		t.Error("down migration should contain DROP TABLE statements")
	}
}
