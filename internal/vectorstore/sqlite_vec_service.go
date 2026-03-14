package vectorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/mattn/go-sqlite3"
)

const (
	defaultSQLiteVecDirName      = "sqlite-vec"
	defaultSQLiteVecDBFileName   = "store.db"
	sqliteVecExtensionBaseName   = "vec0"
	sqliteVecExtensionPathEnvVar = "GOGOCLAW_SQLITE_VEC_PATH"
	defaultProfileName           = "default"
	sqliteVecMetaTableName       = "gogoclaw_vec_service_meta"
	sqliteVecProfilesTableName   = "gogoclaw_vec_profiles"
	sqliteVecTablesTableName     = "gogoclaw_vec_tables"
	sqliteVecObjectPrefix        = "gogoclaw_vec"
)

type StoreKind string

const (
	StoreKindText  StoreKind = "text"
	StoreKindModal StoreKind = "modal"
)

type DistanceMetric string

const (
	DistanceMetricCosine DistanceMetric = "cosine"
	DistanceMetricL2     DistanceMetric = "l2"
)

type UpsertRequest struct {
	StoreKind    StoreKind
	ExternalID   string
	Embedding    []float32
	MetadataJSON string
}

type SearchRequest struct {
	StoreKind  StoreKind
	Query      []float32
	Limit      int
	Metric     DistanceMetric
	ExternalID string
}

type SearchResult struct {
	ExternalID   string
	MetadataJSON string
	Distance     float64
}

var sqliteIdentifierSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

// ThresholdSearchRequest filters search results by a threshold value.
// For cosine metric, Threshold is interpreted as minimum similarity (1-distance);
// for other metrics (e.g. L2), Threshold is interpreted as maximum distance.
type ThresholdSearchRequest struct {
	StoreKind  StoreKind
	Query      []float32
	Metric     DistanceMetric
	MaxResults int
	Threshold  float64
	ExternalID string
}

type DeleteRequest struct {
	StoreKind  StoreKind
	ExternalID string
}

type Service interface {
	Start() error
	Stop() error
	Path() string
	DB() *sql.DB
	Upsert(request UpsertRequest) error
	Delete(request DeleteRequest) error
	SearchTopK(request SearchRequest) ([]SearchResult, error)
	SearchByThreshold(request ThresholdSearchRequest) ([]SearchResult, error)
}

type sqliteVecService struct {
	dbDir           string
	dbPath          string
	extensionPath   string
	workspace       string
	profileName     string
	embedding       config.EmbeddingProfileConfig
	mu              sync.Mutex
	started         bool
	db              *sql.DB
	extensionLoaded bool
}

type profileStoreDefinition struct {
	ProfileName     string
	StoreKind       string
	OutputDimension int
	VectorTableName string
	MetadataTable   string
}

func NewSQLiteVecService(workspace string, profileName string, embedding config.EmbeddingProfileConfig) Service {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		profileName = defaultProfileName
	}
	dbDir := filepath.Join(workspace, defaultSQLiteVecDirName)
	return &sqliteVecService{
		dbDir:         dbDir,
		dbPath:        filepath.Join(dbDir, defaultSQLiteVecDBFileName),
		extensionPath: resolveSQLiteVecExtensionPath(dbDir),
		workspace:     workspace,
		profileName:   profileName,
		embedding:     embedding,
	}
}

func (service *sqliteVecService) Start() error {
	service.mu.Lock()
	defer service.mu.Unlock()

	if service.started {
		return nil
	}
	if strings.TrimSpace(service.dbPath) == "" {
		return fmt.Errorf("sqlite-vec db path is not configured")
	}
	if err := os.MkdirAll(service.dbDir, 0755); err != nil {
		return fmt.Errorf("create sqlite-vec directory: %w", err)
	}

	db, err := sql.Open("sqlite3", service.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite-vec database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping sqlite-vec database: %w", err)
	}
	loaded, err := service.loadSQLiteVecExtension(db)
	if err != nil {
		_ = db.Close()
		return err
	}
	if err := initializeSQLiteVecMetadata(db); err != nil {
		_ = db.Close()
		return err
	}
	if err := service.initializeProfileSchema(db, loaded); err != nil {
		_ = db.Close()
		return err
	}

	service.db = db
	service.started = true
	service.extensionLoaded = loaded
	return nil
}

func (service *sqliteVecService) Stop() error {
	service.mu.Lock()
	defer service.mu.Unlock()

	var err error
	if service.db != nil {
		err = service.db.Close()
	}
	service.db = nil
	service.started = false
	service.extensionLoaded = false
	if err != nil {
		return fmt.Errorf("close sqlite-vec database: %w", err)
	}
	return nil
}

func (service *sqliteVecService) Path() string {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.dbPath
}

func (service *sqliteVecService) DB() *sql.DB {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.db
}

func (service *sqliteVecService) Upsert(request UpsertRequest) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	if !service.started || service.db == nil {
		return fmt.Errorf("sqlite-vec service is not started")
	}
	store, err := service.loadProfileStoreDefinition(request.StoreKind)
	if err != nil {
		return err
	}
	if err := validateEmbeddingInput(store, request.ExternalID, request.Embedding); err != nil {
		return err
	}

	metadataJSON := normalizeMetadataJSON(request.MetadataJSON)
	embeddingJSON, err := encodeEmbeddingJSON(request.Embedding)
	if err != nil {
		return err
	}

	tx, err := service.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite-vec upsert transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(fmt.Sprintf(`
		insert into %s (external_id, embedding_json, metadata_json, updated_at)
		values (?, ?, ?, current_timestamp)
		on conflict(external_id) do update set
			embedding_json=excluded.embedding_json,
			metadata_json=excluded.metadata_json,
			updated_at=current_timestamp
	`, quoteSQLiteIdentifier(store.MetadataTable)), request.ExternalID, embeddingJSON, metadataJSON); err != nil {
		return fmt.Errorf("upsert sqlite-vec metadata record: %w", err)
	}

	var rowID int64
	if err := tx.QueryRow(fmt.Sprintf(`select rowid from %s where external_id = ?`, quoteSQLiteIdentifier(store.MetadataTable)), request.ExternalID).Scan(&rowID); err != nil {
		return fmt.Errorf("load sqlite-vec metadata rowid: %w", err)
	}

	if service.extensionLoaded && store.OutputDimension > 0 {
		if _, err := tx.Exec(fmt.Sprintf(`delete from %s where rowid = ?`, quoteSQLiteIdentifier(store.VectorTableName)), rowID); err != nil {
			return fmt.Errorf("delete sqlite-vec vector row: %w", err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`insert into %s (rowid, embedding) values (?, ?)`, quoteSQLiteIdentifier(store.VectorTableName)), rowID, embeddingJSON); err != nil {
			return fmt.Errorf("upsert sqlite-vec vector row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite-vec upsert transaction: %w", err)
	}
	return nil
}

func (service *sqliteVecService) Delete(request DeleteRequest) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	if !service.started || service.db == nil {
		return fmt.Errorf("sqlite-vec service is not started")
	}
	store, err := service.loadProfileStoreDefinition(request.StoreKind)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.ExternalID) == "" {
		return fmt.Errorf("sqlite-vec external id is required")
	}

	tx, err := service.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite-vec delete transaction: %w", err)
	}
	defer tx.Rollback()

	var rowID int64
	if err := tx.QueryRow(
		fmt.Sprintf(`select rowid from %s where external_id = ?`, quoteSQLiteIdentifier(store.MetadataTable)),
		request.ExternalID,
	).Scan(&rowID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("load sqlite-vec metadata rowid: %w", err)
	}

	if service.extensionLoaded && store.OutputDimension > 0 {
		if _, err := tx.Exec(fmt.Sprintf(`delete from %s where rowid = ?`, quoteSQLiteIdentifier(store.VectorTableName)), rowID); err != nil {
			return fmt.Errorf("delete sqlite-vec vector row: %w", err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`delete from %s where external_id = ?`, quoteSQLiteIdentifier(store.MetadataTable)), request.ExternalID); err != nil {
		return fmt.Errorf("delete sqlite-vec metadata row: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite-vec delete transaction: %w", err)
	}
	return nil
}

func (service *sqliteVecService) SearchTopK(request SearchRequest) ([]SearchResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	if !service.started || service.db == nil {
		return nil, fmt.Errorf("sqlite-vec service is not started")
	}
	store, err := service.loadProfileStoreDefinition(request.StoreKind)
	if err != nil {
		return nil, err
	}
	if err := validateSearchInput(store, request); err != nil {
		return nil, err
	}
	metric := normalizeDistanceMetric(request.Metric)

	if service.extensionLoaded && store.OutputDimension > 0 && metric == DistanceMetricL2 {
		results, err := service.searchWithSQLiteVec(store, request)
		if err == nil {
			return results, nil
		}
	}

	return service.searchFallback(store, request, metric)
}

func (service *sqliteVecService) SearchByThreshold(request ThresholdSearchRequest) ([]SearchResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	if !service.started || service.db == nil {
		return nil, fmt.Errorf("sqlite-vec service is not started")
	}
	store, err := service.loadProfileStoreDefinition(request.StoreKind)
	if err != nil {
		return nil, err
	}
	if len(request.Query) == 0 {
		return nil, fmt.Errorf("sqlite-vec search query is required")
	}
	if store.OutputDimension > 0 && len(request.Query) != store.OutputDimension {
		return nil, fmt.Errorf("sqlite-vec search dimension mismatch: got %d want %d", len(request.Query), store.OutputDimension)
	}
	metric := normalizeDistanceMetric(request.Metric)
	maxResults := request.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	var candidates []SearchResult
	if service.extensionLoaded && store.OutputDimension > 0 && metric == DistanceMetricL2 {
		candidates, err = service.searchWithSQLiteVec(store, SearchRequest{
			StoreKind:  request.StoreKind,
			Query:      request.Query,
			Limit:      maxResults,
			Metric:     metric,
			ExternalID: request.ExternalID,
		})
	} else {
		candidates, err = service.searchFallback(store, SearchRequest{
			StoreKind:  request.StoreKind,
			Query:      request.Query,
			Limit:      maxResults,
			Metric:     metric,
			ExternalID: request.ExternalID,
		}, metric)
	}
	if err != nil {
		return nil, err
	}

	filtered := make([]SearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if metric == DistanceMetricCosine {
			similarity := 1.0 - candidate.Distance
			if similarity >= request.Threshold {
				filtered = append(filtered, candidate)
			}
		} else {
			if candidate.Distance <= request.Threshold {
				filtered = append(filtered, candidate)
			}
		}
	}
	return filtered, nil
}

func (service *sqliteVecService) loadSQLiteVecExtension(db *sql.DB) (bool, error) {
	if strings.TrimSpace(service.extensionPath) == "" {
		return false, nil
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return false, fmt.Errorf("acquire sqlite connection: %w", err)
	}
	defer conn.Close()

	if err := conn.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected sqlite driver connection type: %T", driverConn)
		}
		if err := sqliteConn.LoadExtension(service.extensionPath, ""); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("load sqlite-vec extension %s: %w", service.extensionPath, err)
	}

	if _, err := db.Exec("select vec_version()"); err != nil {
		return false, fmt.Errorf("verify sqlite-vec extension: %w", err)
	}

	return true, nil
}

func initializeSQLiteVecMetadata(db *sql.DB) error {
	if _, err := db.Exec(`
		create table if not exists gogoclaw_vec_service_meta (
			key text primary key,
			value text not null
		)
	`); err != nil {
		return fmt.Errorf("initialize sqlite-vec metadata: %w", err)
	}
	if _, err := db.Exec(`
		create table if not exists gogoclaw_vec_profiles (
			name text primary key,
			workspace_path text not null,
			text_model text not null default '',
			text_dimensions integer not null default 0,
			modal_model text not null default '',
			modal_dimensions integer not null default 0,
			extension_loaded integer not null default 0,
			created_at text not null default current_timestamp,
			updated_at text not null default current_timestamp
		)
	`); err != nil {
		return fmt.Errorf("initialize sqlite-vec profiles: %w", err)
	}
	if _, err := db.Exec(`
		create table if not exists gogoclaw_vec_tables (
			profile_name text not null,
			store_kind text not null,
			vector_table text not null,
			metadata_table text not null,
			output_dimension integer not null default 0,
			extension_loaded integer not null default 0,
			created_at text not null default current_timestamp,
			updated_at text not null default current_timestamp,
			primary key (profile_name, store_kind),
			foreign key (profile_name) references gogoclaw_vec_profiles(name)
		)
	`); err != nil {
		return fmt.Errorf("initialize sqlite-vec table registry: %w", err)
	}
	return nil
}

func (service *sqliteVecService) initializeProfileSchema(db *sql.DB, extensionLoaded bool) error {
	if strings.TrimSpace(service.profileName) == "" {
		return fmt.Errorf("sqlite-vec profile name is required")
	}

	if _, err := db.Exec(`
		insert into gogoclaw_vec_profiles (
			name, workspace_path, text_model, text_dimensions, modal_model, modal_dimensions, extension_loaded, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, current_timestamp)
		on conflict(name) do update set
			workspace_path=excluded.workspace_path,
			text_model=excluded.text_model,
			text_dimensions=excluded.text_dimensions,
			modal_model=excluded.modal_model,
			modal_dimensions=excluded.modal_dimensions,
			extension_loaded=excluded.extension_loaded,
			updated_at=current_timestamp
	`,
		service.profileName,
		service.workspace,
		service.embedding.Text.Model,
		maxInt(service.embedding.Text.OutputDimension, 0),
		service.embedding.Modal.Model,
		maxInt(service.embedding.Modal.OutputDimension, 0),
		boolToInt(extensionLoaded),
	); err != nil {
		return fmt.Errorf("upsert sqlite-vec profile metadata: %w", err)
	}

	stores := []profileStoreDefinition{
		newProfileStoreDefinition(service.profileName, "text", service.embedding.Text.OutputDimension),
		newProfileStoreDefinition(service.profileName, "modal", service.embedding.Modal.OutputDimension),
	}
	for _, store := range stores {
		if err := ensureSQLiteVecProfileStore(db, store, extensionLoaded); err != nil {
			return err
		}
	}

	return nil
}

func newProfileStoreDefinition(profileName string, storeKind string, outputDimension int) profileStoreDefinition {
	normalizedProfile := normalizeSQLiteIdentifier(profileName)
	normalizedKind := normalizeSQLiteIdentifier(storeKind)
	return profileStoreDefinition{
		ProfileName:     profileName,
		StoreKind:       storeKind,
		OutputDimension: maxInt(outputDimension, 0),
		VectorTableName: fmt.Sprintf("%s_%s_%s_vectors", sqliteVecObjectPrefix, normalizedProfile, normalizedKind),
		MetadataTable:   fmt.Sprintf("%s_%s_%s_records", sqliteVecObjectPrefix, normalizedProfile, normalizedKind),
	}
}

func ensureSQLiteVecProfileStore(db *sql.DB, store profileStoreDefinition, extensionLoaded bool) error {
	if _, err := db.Exec(fmt.Sprintf(`
		create table if not exists %s (
			rowid integer primary key,
			external_id text not null unique,
			embedding_json text not null default '[]',
			metadata_json text not null default '{}',
			created_at text not null default current_timestamp,
			updated_at text not null default current_timestamp
		)
	`, quoteSQLiteIdentifier(store.MetadataTable))); err != nil {
		return fmt.Errorf("initialize sqlite-vec metadata table %s: %w", store.MetadataTable, err)
	}
	if err := ensureMetadataTableColumns(db, store.MetadataTable); err != nil {
		return err
	}

	if _, err := db.Exec(`
		insert into gogoclaw_vec_tables (
			profile_name, store_kind, vector_table, metadata_table, output_dimension, extension_loaded, updated_at
		) values (?, ?, ?, ?, ?, ?, current_timestamp)
		on conflict(profile_name, store_kind) do update set
			vector_table=excluded.vector_table,
			metadata_table=excluded.metadata_table,
			output_dimension=excluded.output_dimension,
			extension_loaded=excluded.extension_loaded,
			updated_at=current_timestamp
	`,
		store.ProfileName,
		store.StoreKind,
		store.VectorTableName,
		store.MetadataTable,
		store.OutputDimension,
		boolToInt(extensionLoaded),
	); err != nil {
		return fmt.Errorf("upsert sqlite-vec table registry for %s/%s: %w", store.ProfileName, store.StoreKind, err)
	}

	if !extensionLoaded || store.OutputDimension <= 0 {
		return nil
	}

	if _, err := db.Exec(fmt.Sprintf(`create virtual table if not exists %s using vec0(embedding float[%d])`, quoteSQLiteIdentifier(store.VectorTableName), store.OutputDimension)); err != nil {
		return fmt.Errorf("initialize sqlite-vec virtual table %s: %w", store.VectorTableName, err)
	}

	return nil
}

func ensureMetadataTableColumns(db *sql.DB, tableName string) error {
	columns, err := loadTableColumns(db, tableName)
	if err != nil {
		return err
	}
	if _, ok := columns["embedding_json"]; !ok {
		if _, err := db.Exec(fmt.Sprintf(`alter table %s add column embedding_json text not null default '[]'`, quoteSQLiteIdentifier(tableName))); err != nil {
			return fmt.Errorf("add embedding_json to %s: %w", tableName, err)
		}
	}
	return nil
}

func loadTableColumns(db *sql.DB, tableName string) (map[string]struct{}, error) {
	rows, err := db.Query(fmt.Sprintf(`pragma table_info(%s)`, quoteSQLiteIdentifier(tableName)))
	if err != nil {
		return nil, fmt.Errorf("load table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan table info for %s: %w", tableName, err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table info for %s: %w", tableName, err)
	}
	return columns, nil
}

func (service *sqliteVecService) loadProfileStoreDefinition(kind StoreKind) (profileStoreDefinition, error) {
	storeKind := normalizeStoreKind(kind)
	var store profileStoreDefinition
	if err := service.db.QueryRow(`
		select profile_name, store_kind, output_dimension, vector_table, metadata_table
		from gogoclaw_vec_tables
		where profile_name = ? and store_kind = ?
	`, service.profileName, string(storeKind)).Scan(
		&store.ProfileName,
		&store.StoreKind,
		&store.OutputDimension,
		&store.VectorTableName,
		&store.MetadataTable,
	); err != nil {
		return profileStoreDefinition{}, fmt.Errorf("load sqlite-vec store definition for %s/%s: %w", service.profileName, storeKind, err)
	}
	return store, nil
}

func (service *sqliteVecService) searchWithSQLiteVec(store profileStoreDefinition, request SearchRequest) ([]SearchResult, error) {
	query, args, err := buildSQLiteVecSearchQuery(store, request)
	if err != nil {
		return nil, err
	}
	rows, err := service.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search sqlite-vec virtual table: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, request.Limit)
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ExternalID, &result.MetadataJSON, &result.Distance); err != nil {
			return nil, fmt.Errorf("scan sqlite-vec search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite-vec search results: %w", err)
	}
	return results, nil
}

func buildSQLiteVecSearchQuery(store profileStoreDefinition, request SearchRequest) (string, []any, error) {
	queryJSON, err := encodeEmbeddingJSON(request.Query)
	if err != nil {
		return "", nil, err
	}
	query := fmt.Sprintf(`
		select m.external_id, m.metadata_json, v.distance
		from %s as v
		join %s as m on m.rowid = v.rowid
		where embedding match ?
	`, quoteSQLiteIdentifier(store.VectorTableName), quoteSQLiteIdentifier(store.MetadataTable))
	args := []any{queryJSON}
	if strings.TrimSpace(request.ExternalID) != "" {
		query += " and m.external_id <> ?\n"
		args = append(args, request.ExternalID)
	}
	query += "\t\torder by distance\n\t\tlimit ?\n\t"
	args = append(args, request.Limit)
	return query, args, nil
}

func (service *sqliteVecService) searchFallback(store profileStoreDefinition, request SearchRequest, metric DistanceMetric) ([]SearchResult, error) {
	query := fmt.Sprintf(`select external_id, metadata_json, embedding_json from %s`, quoteSQLiteIdentifier(store.MetadataTable))
	args := make([]any, 0, 1)
	if strings.TrimSpace(request.ExternalID) != "" {
		query += ` where external_id <> ?`
		args = append(args, request.ExternalID)
	}
	rows, err := service.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite-vec fallback candidates: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var result SearchResult
		var embeddingJSON string
		if err := rows.Scan(&result.ExternalID, &result.MetadataJSON, &embeddingJSON); err != nil {
			return nil, fmt.Errorf("scan sqlite-vec fallback row: %w", err)
		}
		embedding, err := decodeEmbeddingJSON(embeddingJSON)
		if err != nil {
			return nil, err
		}
		if len(embedding) != len(request.Query) {
			continue
		}
		result.Distance, err = calculateDistance(metric, request.Query, embedding)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite-vec fallback rows: %w", err)
	}

	sort.Slice(results, func(i int, j int) bool { return results[i].Distance < results[j].Distance })
	if len(results) > request.Limit {
		results = results[:request.Limit]
	}
	return results, nil
}

func resolveSQLiteVecExtensionPath(dbDir string) string {
	if override := strings.TrimSpace(os.Getenv(sqliteVecExtensionPathEnvVar)); override != "" {
		return trimSQLiteExtensionSuffix(override)
	}

	searchRoots := []string{dbDir}
	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		searchRoots = append(searchRoots,
			executableDir,
			filepath.Join(executableDir, defaultSQLiteVecDirName),
			filepath.Join(executableDir, "lib", defaultSQLiteVecDirName),
		)
	}

	for _, root := range searchRoots {
		if resolved, ok := findSQLiteVecExtension(root); ok {
			return resolved
		}
	}

	return ""
}

func findSQLiteVecExtension(root string) (string, bool) {
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	for _, candidate := range sqliteVecExtensionCandidates(root) {
		if _, err := os.Stat(candidate); err == nil {
			return trimSQLiteExtensionSuffix(candidate), true
		}
	}
	return "", false
}

func sqliteVecExtensionCandidates(root string) []string {
	base := filepath.Join(root, sqliteVecExtensionBaseName)
	return []string{
		base,
		base + ".dylib",
		base + ".so",
		base + ".dll",
	}
}

func trimSQLiteExtensionSuffix(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".dylib", ".so", ".dll":
		return strings.TrimSuffix(path, ext)
	default:
		return path
	}
}

func normalizeSQLiteIdentifier(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = sqliteIdentifierSanitizer.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return defaultProfileName
	}
	return normalized
}

func quoteSQLiteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func maxInt(value int, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func normalizeStoreKind(kind StoreKind) StoreKind {
	switch kind {
	case StoreKindModal:
		return StoreKindModal
	default:
		return StoreKindText
	}
}

func normalizeDistanceMetric(metric DistanceMetric) DistanceMetric {
	switch metric {
	case DistanceMetricL2:
		return DistanceMetricL2
	default:
		return DistanceMetricCosine
	}
}

func validateEmbeddingInput(store profileStoreDefinition, externalID string, embedding []float32) error {
	if strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("sqlite-vec external id is required")
	}
	if len(embedding) == 0 {
		return fmt.Errorf("sqlite-vec embedding is required")
	}
	if store.OutputDimension > 0 && len(embedding) != store.OutputDimension {
		return fmt.Errorf("sqlite-vec embedding dimension mismatch: got %d want %d", len(embedding), store.OutputDimension)
	}
	return nil
}

func validateSearchInput(store profileStoreDefinition, request SearchRequest) error {
	if request.Limit <= 0 {
		return fmt.Errorf("sqlite-vec search limit must be greater than zero")
	}
	if len(request.Query) == 0 {
		return fmt.Errorf("sqlite-vec search query is required")
	}
	if store.OutputDimension > 0 && len(request.Query) != store.OutputDimension {
		return fmt.Errorf("sqlite-vec search dimension mismatch: got %d want %d", len(request.Query), store.OutputDimension)
	}
	return nil
}

func normalizeMetadataJSON(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func encodeEmbeddingJSON(embedding []float32) (string, error) {
	encoded, err := json.Marshal(embedding)
	if err != nil {
		return "", fmt.Errorf("encode sqlite-vec embedding: %w", err)
	}
	return string(encoded), nil
}

func decodeEmbeddingJSON(value string) ([]float32, error) {
	var embedding []float32
	if err := json.Unmarshal([]byte(value), &embedding); err != nil {
		return nil, fmt.Errorf("decode sqlite-vec embedding: %w", err)
	}
	return embedding, nil
}

func calculateDistance(metric DistanceMetric, left []float32, right []float32) (float64, error) {
	if len(left) != len(right) {
		return 0, fmt.Errorf("sqlite-vec distance dimension mismatch: %d vs %d", len(left), len(right))
	}
	switch metric {
	case DistanceMetricL2:
		var sum float64
		for i := range left {
			delta := float64(left[i] - right[i])
			sum += delta * delta
		}
		return math.Sqrt(sum), nil
	default:
		var dot float64
		var leftNorm float64
		var rightNorm float64
		for i := range left {
			leftValue := float64(left[i])
			rightValue := float64(right[i])
			dot += leftValue * rightValue
			leftNorm += leftValue * leftValue
			rightNorm += rightValue * rightValue
		}
		if leftNorm == 0 || rightNorm == 0 {
			return 1, nil
		}
		return 1 - dot/(math.Sqrt(leftNorm)*math.Sqrt(rightNorm)), nil
	}
}
