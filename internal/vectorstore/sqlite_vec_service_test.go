package vectorstore

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
)

func TestSQLiteVecServiceStartCreatesDatabase(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{})

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, defaultSQLiteVecDirName, defaultSQLiteVecDBFileName)); err != nil {
		t.Fatalf("sqlite-vec db file not created: %v", err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(workspace, defaultSQLiteVecDirName, defaultSQLiteVecDBFileName))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	assertTableExists(t, db, "sqlite_vec_profiles")
	assertTableExists(t, db, "sqlite_vec_tables")
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSQLiteVecServiceStartIsIdempotent(t *testing.T) {
	service := NewSQLiteVecService(t.TempDir(), "default", config.EmbeddingProfileConfig{})

	if err := service.Start(); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := service.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSQLiteVecServiceInitializesProfileRegistryAndMetadataTables(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "my-profile", config.EmbeddingProfileConfig{
		Text:  config.EmbeddingModelConfig{Model: "voyage-4-large", OutputDimension: 1024},
		Modal: config.EmbeddingModelConfig{Model: "voyage-multimodal-3.5", OutputDimension: 1024},
	})

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	db, err := sql.Open("sqlite3", filepath.Join(workspace, defaultSQLiteVecDirName, defaultSQLiteVecDBFileName))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	assertTableExists(t, db, "sqlite_vec_my_profile_text_records")
	assertTableExists(t, db, "sqlite_vec_my_profile_modal_records")

	var textDim int
	if err := db.QueryRow(`select text_dimensions from sqlite_vec_profiles where name = ?`, "my-profile").Scan(&textDim); err != nil {
		t.Fatalf("QueryRow(text_dimensions) error = %v", err)
	}
	if textDim != 1024 {
		t.Fatalf("text_dimensions = %d, want 1024", textDim)
	}

	var vectorTable string
	if err := db.QueryRow(`select vector_table from sqlite_vec_tables where profile_name = ? and store_kind = ?`, "my-profile", "text").Scan(&vectorTable); err != nil {
		t.Fatalf("QueryRow(vector_table) error = %v", err)
	}
	if vectorTable != "sqlite_vec_my_profile_text_vectors" {
		t.Fatalf("vector_table = %q, want sqlite_vec_my_profile_text_vectors", vectorTable)
	}
}

func TestResolveSQLiteVecExtensionPathPrefersEnvironmentOverride(t *testing.T) {
	t.Setenv(sqliteVecExtensionPathEnvVar, "/tmp/sqlite-vec/vec0.dylib")

	resolved := resolveSQLiteVecExtensionPath(t.TempDir())
	if resolved != "/tmp/sqlite-vec/vec0" {
		t.Fatalf("resolveSQLiteVecExtensionPath() = %q, want /tmp/sqlite-vec/vec0", resolved)
	}
}

func TestResolveSQLiteVecExtensionPathFindsWorkspaceArtifact(t *testing.T) {
	workspace := t.TempDir()
	extensionDir := filepath.Join(workspace, defaultSQLiteVecDirName)
	if err := os.MkdirAll(extensionDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	extensionPath := filepath.Join(extensionDir, sqliteVecExtensionBaseName+".dylib")
	if err := os.WriteFile(extensionPath, []byte("stub"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	resolved := resolveSQLiteVecExtensionPath(extensionDir)
	if resolved != filepath.Join(extensionDir, sqliteVecExtensionBaseName) {
		t.Fatalf("resolveSQLiteVecExtensionPath() = %q, want %q", resolved, filepath.Join(extensionDir, sqliteVecExtensionBaseName))
	}
}

func assertTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	var exists string
	if err := db.QueryRow(`select name from sqlite_master where type = 'table' and name = ?`, tableName).Scan(&exists); err != nil {
		t.Fatalf("table %s missing: %v", tableName, err)
	}
}
