package vectorstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
)

var sqliteIdentifierSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

type Service interface {
	Start() error
	Stop() error
	Path() string
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
		create table if not exists sqlite_vec_service_meta (
			key text primary key,
			value text not null
		)
	`); err != nil {
		return fmt.Errorf("initialize sqlite-vec metadata: %w", err)
	}
	if _, err := db.Exec(`
		create table if not exists sqlite_vec_profiles (
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
		create table if not exists sqlite_vec_tables (
			profile_name text not null,
			store_kind text not null,
			vector_table text not null,
			metadata_table text not null,
			output_dimension integer not null default 0,
			extension_loaded integer not null default 0,
			created_at text not null default current_timestamp,
			updated_at text not null default current_timestamp,
			primary key (profile_name, store_kind),
			foreign key (profile_name) references sqlite_vec_profiles(name)
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
		insert into sqlite_vec_profiles (
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
		VectorTableName: fmt.Sprintf("sqlite_vec_%s_%s_vectors", normalizedProfile, normalizedKind),
		MetadataTable:   fmt.Sprintf("sqlite_vec_%s_%s_records", normalizedProfile, normalizedKind),
	}
}

func ensureSQLiteVecProfileStore(db *sql.DB, store profileStoreDefinition, extensionLoaded bool) error {
	if _, err := db.Exec(fmt.Sprintf(`
		create table if not exists %s (
			rowid integer primary key,
			external_id text not null unique,
			metadata_json text not null default '{}',
			created_at text not null default current_timestamp,
			updated_at text not null default current_timestamp
		)
	`, quoteSQLiteIdentifier(store.MetadataTable))); err != nil {
		return fmt.Errorf("initialize sqlite-vec metadata table %s: %w", store.MetadataTable, err)
	}

	if _, err := db.Exec(`
		insert into sqlite_vec_tables (
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
