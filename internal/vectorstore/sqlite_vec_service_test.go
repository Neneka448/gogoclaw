package vectorstore

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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
	assertTableExists(t, db, sqliteVecProfilesTableName)
	assertTableExists(t, db, sqliteVecTablesTableName)
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

	assertTableExists(t, db, "gogoclaw_vec_my_profile_text_records")
	assertTableExists(t, db, "gogoclaw_vec_my_profile_modal_records")

	var textDim int
	if err := db.QueryRow(`select text_dimensions from gogoclaw_vec_profiles where name = ?`, "my-profile").Scan(&textDim); err != nil {
		t.Fatalf("QueryRow(text_dimensions) error = %v", err)
	}
	if textDim != 1024 {
		t.Fatalf("text_dimensions = %d, want 1024", textDim)
	}

	var vectorTable string
	if err := db.QueryRow(`select vector_table from gogoclaw_vec_tables where profile_name = ? and store_kind = ?`, "my-profile", "text").Scan(&vectorTable); err != nil {
		t.Fatalf("QueryRow(vector_table) error = %v", err)
	}
	if vectorTable != "gogoclaw_vec_my_profile_text_vectors" {
		t.Fatalf("vector_table = %q, want gogoclaw_vec_my_profile_text_vectors", vectorTable)
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

func TestSQLiteVecServiceUpsertAndSearchTopKFallback(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	fixtures := []UpsertRequest{
		{StoreKind: StoreKindText, ExternalID: "alpha", Embedding: []float32{1, 0, 0}, MetadataJSON: `{"title":"alpha"}`},
		{StoreKind: StoreKindText, ExternalID: "beta", Embedding: []float32{0.9, 0.1, 0}, MetadataJSON: `{"title":"beta"}`},
		{StoreKind: StoreKindText, ExternalID: "gamma", Embedding: []float32{0, 1, 0}, MetadataJSON: `{"title":"gamma"}`},
	}
	for _, fixture := range fixtures {
		if err := service.Upsert(fixture); err != nil {
			t.Fatalf("Upsert(%s) error = %v", fixture.ExternalID, err)
		}
	}

	results, err := service.SearchTopK(SearchRequest{
		StoreKind: StoreKindText,
		Query:     []float32{1, 0, 0},
		Limit:     2,
		Metric:    DistanceMetricCosine,
	})
	if err != nil {
		t.Fatalf("SearchTopK() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ExternalID != "alpha" {
		t.Fatalf("results[0].ExternalID = %q, want alpha", results[0].ExternalID)
	}
	if results[1].ExternalID != "beta" {
		t.Fatalf("results[1].ExternalID = %q, want beta", results[1].ExternalID)
	}
}

func TestSQLiteVecServiceDeleteRemovesEmbedding(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	if err := service.Upsert(UpsertRequest{
		StoreKind:  StoreKindText,
		ExternalID: "alpha",
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if err := service.Delete(DeleteRequest{
		StoreKind:  StoreKindText,
		ExternalID: "alpha",
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	results, err := service.SearchTopK(SearchRequest{
		StoreKind: StoreKindText,
		Query:     []float32{1, 0, 0},
		Limit:     1,
		Metric:    DistanceMetricCosine,
	})
	if err != nil {
		t.Fatalf("SearchTopK() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestSQLiteVecServiceUpsertRejectsDimensionMismatch(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	err := service.Upsert(UpsertRequest{StoreKind: StoreKindText, ExternalID: "bad", Embedding: []float32{1, 2}})
	if err == nil {
		t.Fatal("Upsert() error = nil, want dimension mismatch error")
	}
}

func TestSQLiteVecServiceSearchRejectsBadLimit(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	_, err := service.SearchTopK(SearchRequest{StoreKind: StoreKindText, Query: []float32{1, 0, 0}, Limit: 0})
	if err == nil {
		t.Fatal("SearchTopK() error = nil, want invalid limit error")
	}
}

func TestSQLiteVecServiceSearchByThresholdCosine(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	fixtures := []UpsertRequest{
		{StoreKind: StoreKindText, ExternalID: "a", Embedding: []float32{1, 0, 0}, MetadataJSON: `{}`},
		{StoreKind: StoreKindText, ExternalID: "b", Embedding: []float32{0.9, 0.1, 0}, MetadataJSON: `{}`},
		{StoreKind: StoreKindText, ExternalID: "c", Embedding: []float32{0, 1, 0}, MetadataJSON: `{}`},
	}
	for _, f := range fixtures {
		if err := service.Upsert(f); err != nil {
			t.Fatalf("Upsert(%s) error = %v", f.ExternalID, err)
		}
	}

	// High threshold: only the very similar vectors should pass
	results, err := service.SearchByThreshold(ThresholdSearchRequest{
		StoreKind:  StoreKindText,
		Query:      []float32{1, 0, 0},
		Metric:     DistanceMetricCosine,
		MaxResults: 10,
		Threshold:  0.95,
	})
	if err != nil {
		t.Fatalf("SearchByThreshold() error = %v", err)
	}
	// "a" has similarity=1.0 and "b" is close (~0.99), "c" is far (~0.0)
	for _, r := range results {
		if r.ExternalID == "c" {
			t.Fatal("expected 'c' to be filtered out at threshold 0.95")
		}
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result above threshold 0.95")
	}
}

func TestSQLiteVecServiceSearchByThresholdExcludesExternalID(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	fixtures := []UpsertRequest{
		{StoreKind: StoreKindText, ExternalID: "x", Embedding: []float32{1, 0, 0}, MetadataJSON: `{}`},
		{StoreKind: StoreKindText, ExternalID: "y", Embedding: []float32{1, 0, 0}, MetadataJSON: `{}`},
	}
	for _, f := range fixtures {
		if err := service.Upsert(f); err != nil {
			t.Fatalf("Upsert(%s) error = %v", f.ExternalID, err)
		}
	}

	results, err := service.SearchByThreshold(ThresholdSearchRequest{
		StoreKind:  StoreKindText,
		Query:      []float32{1, 0, 0},
		Metric:     DistanceMetricCosine,
		MaxResults: 10,
		Threshold:  0.5,
		ExternalID: "x",
	})
	if err != nil {
		t.Fatalf("SearchByThreshold() error = %v", err)
	}
	for _, r := range results {
		if r.ExternalID == "x" {
			t.Fatal("expected ExternalID 'x' to be excluded from results")
		}
	}
}

func TestSQLiteVecServiceSearchByThresholdRespectsMaxResults(t *testing.T) {
	workspace := t.TempDir()
	service := NewSQLiteVecService(workspace, "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	fixtures := []UpsertRequest{
		{StoreKind: StoreKindText, ExternalID: "p", Embedding: []float32{1, 0, 0}, MetadataJSON: `{}`},
		{StoreKind: StoreKindText, ExternalID: "q", Embedding: []float32{0.99, 0.01, 0}, MetadataJSON: `{}`},
		{StoreKind: StoreKindText, ExternalID: "r", Embedding: []float32{0.98, 0.02, 0}, MetadataJSON: `{}`},
	}
	for _, f := range fixtures {
		if err := service.Upsert(f); err != nil {
			t.Fatalf("Upsert(%s) error = %v", f.ExternalID, err)
		}
	}

	// All three are very similar, but MaxResults=1 via searchFallback limits candidates
	results, err := service.SearchByThreshold(ThresholdSearchRequest{
		StoreKind:  StoreKindText,
		Query:      []float32{1, 0, 0},
		Metric:     DistanceMetricCosine,
		MaxResults: 1,
		Threshold:  0.5,
	})
	if err != nil {
		t.Fatalf("SearchByThreshold() error = %v", err)
	}
	if len(results) > 1 {
		t.Fatalf("len(results) = %d, want <= 1", len(results))
	}
}

func TestSQLiteVecServiceSearchByThresholdNotStarted(t *testing.T) {
	service := NewSQLiteVecService(t.TempDir(), "default", config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{OutputDimension: 3},
	}).(*sqliteVecService)
	service.extensionPath = ""

	_, err := service.SearchByThreshold(ThresholdSearchRequest{
		StoreKind: StoreKindText,
		Query:     []float32{1, 0, 0},
		Threshold: 0.5,
	})
	if err == nil {
		t.Fatal("SearchByThreshold() error = nil, want not-started error")
	}
}

func TestBuildSQLiteVecSearchQueryExcludesExternalID(t *testing.T) {
	query, args, err := buildSQLiteVecSearchQuery(profileStoreDefinition{
		VectorTableName: "gogoclaw_vec_default_text_vectors",
		MetadataTable:   "gogoclaw_vec_default_text_records",
	}, SearchRequest{
		Query:      []float32{1, 0, 0},
		Limit:      5,
		ExternalID: "alpha",
	})
	if err != nil {
		t.Fatalf("buildSQLiteVecSearchQuery() error = %v", err)
	}
	if !strings.Contains(query, "and m.external_id <> ?") {
		t.Fatalf("query = %q, want external_id exclusion clause", query)
	}
	if len(args) != 3 {
		t.Fatalf("len(args) = %d, want 3", len(args))
	}
	if externalID, ok := args[1].(string); !ok || externalID != "alpha" {
		t.Fatalf("args[1] = %#v, want alpha", args[1])
	}
	if limit, ok := args[2].(int); !ok || limit != 5 {
		t.Fatalf("args[2] = %#v, want 5", args[2])
	}
}

func TestBuildSQLiteVecSearchQueryWithoutExternalID(t *testing.T) {
	query, args, err := buildSQLiteVecSearchQuery(profileStoreDefinition{
		VectorTableName: "gogoclaw_vec_default_text_vectors",
		MetadataTable:   "gogoclaw_vec_default_text_records",
	}, SearchRequest{
		Query: []float32{1, 0, 0},
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("buildSQLiteVecSearchQuery() error = %v", err)
	}
	if strings.Contains(query, "external_id <>") {
		t.Fatalf("query = %q, want no external_id exclusion clause", query)
	}
	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if limit, ok := args[1].(int); !ok || limit != 3 {
		t.Fatalf("args[1] = %#v, want 3", args[1])
	}
}

func assertTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	var exists string
	if err := db.QueryRow(`select name from sqlite_master where type = 'table' and name = ?`, tableName).Scan(&exists); err != nil {
		t.Fatalf("table %s missing: %v", tableName, err)
	}
}
