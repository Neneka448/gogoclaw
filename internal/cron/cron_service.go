package cron

import (
	"errors"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/utils"
	ccron "github.com/robfig/cron/v3"
)

const (
	cronsDirName          = "crons"
	configFileName        = "config.json"
	taskFileName          = "task.md"
	manifestFileName      = "manifest.json"
	sessionRefFileName    = "session.json"
	executionPrefix       = "task_exec_"
	executionTimeFormat   = "20060102T150405Z0700"
	defaultCronChannelID  = "cron"
	defaultCronMessageTyp = "cron"
)

var cronIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var cronNow = time.Now

func CronNowForTest(now func() time.Time) func() {
	previous := cronNow
	cronNow = now
	return func() {
		cronNow = previous
	}
}

type ExecutionRequest struct {
	CronID       string
	SessionID    string
	Prompt       string
	ExecutionDir string
	Metadata     map[string]string
	ProfileName  string
	Mode         string
}

type Executor func(request ExecutionRequest) error

type Service interface {
	EnsureRoot() error
	LoadAll() error
	Start() error
	Stop() error
	ListCrons() ([]StoredCron, error)
	GetCron(cronID string) (*StoredCron, error)
	CreateCron(input UpsertCronInput) (*StoredCron, error)
	UpdateCron(input UpsertCronInput) (*StoredCron, error)
	DeleteCron(cronID string) error
	ExecuteCron(cronID string) error
}

type UpsertCronInput struct {
	CronID         string
	CronExpression string
	Enabled        bool
	Task           string
	ProfileName    string
	InvocationMode string
}

type StoredCron struct {
	Config Config
	Task   string
	Path   string
}

type ExecutionManifest struct {
	CronID      string   `json:"cronID"`
	ExecutionID string   `json:"executionID"`
	Status      string   `json:"status"`
	StartedAt   string   `json:"startedAt"`
	FinishedAt  string   `json:"finishedAt,omitempty"`
	SessionID   string   `json:"sessionID"`
	SessionFile string   `json:"sessionFile"`
	Artifacts   []string `json:"artifacts,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type sessionReference struct {
	SessionID   string `json:"sessionID"`
	SessionFile string `json:"sessionFile"`
}

// cronService is the single cron management layer. It knows all
// workspaces via the ProfileResolver and handles CRUD, scheduling,
// and execution for every cron across every workspace.
type cronService struct {
	resolver *config.ProfileResolver
	manager  CronManager
	executor Executor
	location *time.Location
}

type workspaceCron struct {
	config  Config
	execute func() error
}

type storedCronEntry struct {
	workspace string
	stored    StoredCron
}

func NewCronService(resolver *config.ProfileResolver, manager CronManager, executor Executor, location *time.Location) Service {
	if location == nil {
		location = time.Local
	}
	return &cronService{
		resolver: resolver,
		manager:  manager,
		executor: executor,
		location: location,
	}
}

func (cronTask *workspaceCron) Execute() error {
	return cronTask.execute()
}

func (cronTask *workspaceCron) GetCronConfig() *Config {
	cfg := cronTask.config
	return &cfg
}

// --- Service interface ---

func (s *cronService) EnsureRoot() error {
	for _, workspace := range s.resolver.Workspaces() {
		if err := ensureCronRoot(workspace); err != nil {
			return err
		}
	}
	return nil
}

func (s *cronService) LoadAll() error {
	entries, err := s.listAllEntries()
	if err != nil {
		return err
	}
	if s.manager == nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.stored.Config.Enabled {
			continue
		}
		if err := s.manager.RegisterCron(s.buildRuntimeCron(entry.stored)); err != nil {
			return err
		}
	}
	return nil
}

func (s *cronService) Start() error {
	if s.manager == nil {
		return nil
	}
	return s.manager.Start()
}

func (s *cronService) Stop() error {
	if s.manager == nil {
		return nil
	}
	return s.manager.Stop()
}

func (s *cronService) ListCrons() ([]StoredCron, error) {
	entries, err := s.listAllEntries()
	if err != nil {
		return nil, err
	}
	crons := make([]StoredCron, 0, len(entries))
	for _, entry := range entries {
		crons = append(crons, entry.stored)
	}
	return crons, nil
}

func (s *cronService) GetCron(cronID string) (*StoredCron, error) {
	_, stored, err := s.findCronOwner(cronID)
	return stored, err
}

func (s *cronService) CreateCron(input UpsertCronInput) (*StoredCron, error) {
	resolvedProfile, workspace, err := s.resolver.ResolveWorkspace(input.ProfileName)
	if err != nil {
		return nil, err
	}
	if ownerWS, existing, findErr := s.findCronOwner(strings.TrimSpace(input.CronID)); findErr == nil {
		return nil, fmt.Errorf("cron already exists in workspace %s for profile %s: %s", ownerWS, existing.Config.ProfileName, existing.Config.CronID)
	} else if !isCronNotFound(findErr) {
		return nil, findErr
	}
	input.ProfileName = resolvedProfile
	storedCron, err := normalizeCronInput(workspace, input)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(storedCron.Path); err == nil {
		return nil, fmt.Errorf("cron already exists: %s", storedCron.Config.CronID)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := writeCronFiles(workspace, *storedCron); err != nil {
		return nil, err
	}
	if err := s.syncRuntime(*storedCron); err != nil {
		return nil, err
	}
	return readCronFiles(workspace, storedCron.Config.CronID)
}

func (s *cronService) UpdateCron(input UpsertCronInput) (*StoredCron, error) {
	resolvedProfile, workspace, err := s.resolver.ResolveWorkspace(input.ProfileName)
	if err != nil {
		return nil, err
	}
	ownerWS, _, ownerErr := s.findCronOwner(strings.TrimSpace(input.CronID))
	if ownerErr != nil && !isCronNotFound(ownerErr) {
		return nil, ownerErr
	}
	if ownerWS != "" && ownerWS != workspace {
		return nil, fmt.Errorf("cron %s belongs to workspace %s and cannot be moved to %s", strings.TrimSpace(input.CronID), ownerWS, workspace)
	}
	input.ProfileName = resolvedProfile
	storedCron, err := normalizeCronInput(workspace, input)
	if err != nil {
		return nil, err
	}
	if _, err := readCronFiles(workspace, storedCron.Config.CronID); err != nil {
		return nil, err
	}
	if err := writeCronFiles(workspace, *storedCron); err != nil {
		return nil, err
	}
	if err := s.syncRuntime(*storedCron); err != nil {
		return nil, err
	}
	return readCronFiles(workspace, storedCron.Config.CronID)
}

func (s *cronService) DeleteCron(cronID string) error {
	ownerWS, _, err := s.findCronOwner(cronID)
	if err != nil {
		return err
	}
	cronID = strings.TrimSpace(cronID)
	if s.manager != nil {
		if err := s.manager.DeleteCron(cronID); err != nil && !isCronNotFound(err) {
			return err
		}
	}
	dir, err := resolveCronDir(ownerWS, cronID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (s *cronService) ExecuteCron(cronID string) error {
	_, stored, err := s.findCronOwner(cronID)
	if err != nil {
		return err
	}
	return s.executeCron(*stored)
}

// --- Internal methods ---

func (s *cronService) executeCron(storedCron StoredCron) error {
	if !storedCron.Config.Enabled {
		return fmt.Errorf("cron is disabled: %s", storedCron.Config.CronID)
	}
	if s.executor == nil {
		return fmt.Errorf("cron executor is not configured")
	}

	lockPath := filepath.Join(storedCron.Path, ".lock")
	lock, lockErr := utils.AcquireFileLock(utils.FileLockOptions{
		Path:     lockPath,
		Resource: fmt.Sprintf("cron:%s", storedCron.Config.CronID),
		Metadata: map[string]string{
			"cron_id":     storedCron.Config.CronID,
			"workspace":   filepath.Dir(filepath.Dir(storedCron.Path)),
			"expression":  storedCron.Config.CronExpression,
			"profile_name": storedCron.Config.ProfileName,
		},
		Now: s.currentTime,
	})
	if lockErr != nil {
		var heldErr *utils.FileLockHeldError
		if errors.As(lockErr, &heldErr) {
			fmt.Fprintf(os.Stderr, "[cron] skipping %s (locked%s)\n", storedCron.Config.CronID, utils.FormatFileLockInfo(heldErr.Info))
			return nil
		}
		return lockErr
	}
	defer func() {
		if err := lock.Release(); err != nil {
			fmt.Fprintf(os.Stderr, "[cron] release lock %s error: %v\n", storedCron.Config.CronID, err)
		}
	}()

	fmt.Fprintf(os.Stderr, "[cron] executing %s (%s)\n", storedCron.Config.CronID, storedCron.Config.CronExpression)

	startedAt := s.currentTime()
	executionID := executionPrefix + startedAt.Format(executionTimeFormat)
	executionDir := filepath.Join(storedCron.Path, executionID)
	if err := os.MkdirAll(executionDir, 0755); err != nil {
		return err
	}
	sessionID := buildCronSessionID(storedCron.Config.CronID, executionID)
	sessionFile := filepath.ToSlash(filepath.Join("sessions", sessionID+".json"))
	manifest := ExecutionManifest{
		CronID:      storedCron.Config.CronID,
		ExecutionID: executionID,
		Status:      "running",
		StartedAt:   startedAt.Format(time.RFC3339),
		SessionID:   sessionID,
		SessionFile: sessionFile,
	}
	if err := writeJSON(filepath.Join(executionDir, manifestFileName), manifest); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(executionDir, sessionRefFileName), sessionReference{SessionID: sessionID, SessionFile: sessionFile}); err != nil {
		return err
	}

	execErr := s.executor(ExecutionRequest{
		CronID:       storedCron.Config.CronID,
		SessionID:    sessionID,
		Prompt:       buildExecutionPrompt(&storedCron, executionDir),
		ExecutionDir: executionDir,
		Metadata: map[string]string{
			"source":  defaultCronChannelID,
			"cron_id": storedCron.Config.CronID,
			"exec_id": executionID,
		},
		ProfileName: storedCron.Config.ProfileName,
		Mode:        storedCron.Config.InvocationMode,
	})

	manifest.Status = "succeeded"
	if execErr != nil {
		manifest.Status = "failed"
		manifest.Error = execErr.Error()
	}
	manifest.FinishedAt = s.currentTime().Format(time.RFC3339)
	artifacts, artifactErr := collectArtifacts(executionDir)
	if artifactErr != nil && execErr == nil {
		execErr = artifactErr
		manifest.Status = "failed"
		manifest.Error = artifactErr.Error()
	}
	manifest.Artifacts = artifacts
	if err := writeJSON(filepath.Join(executionDir, manifestFileName), manifest); err != nil {
		return err
	}
	return execErr
}

func (s *cronService) buildRuntimeCron(storedCron StoredCron) Cron {
	cronConfig := storedCron.Config
	return &workspaceCron{
		config: cronConfig,
		execute: func() error {
			return s.ExecuteCron(cronConfig.CronID)
		},
	}
}

func (s *cronService) currentTime() time.Time {
	now := cronNow()
	if s.location == nil {
		return now
	}
	return now.In(s.location)
}

func (s *cronService) syncRuntime(storedCron StoredCron) error {
	if s.manager == nil {
		return nil
	}
	if err := s.manager.RegisterCron(s.buildRuntimeCron(storedCron)); err != nil {
		return err
	}
	if !storedCron.Config.Enabled {
		return s.manager.DeleteCron(storedCron.Config.CronID)
	}
	return nil
}

func (s *cronService) listAllEntries() ([]storedCronEntry, error) {
	var entries []storedCronEntry
	seen := make(map[string]string)
	for _, workspace := range s.resolver.Workspaces() {
		storedCrons, err := listWorkspaceCrons(workspace)
		if err != nil {
			return nil, err
		}
		for _, storedCron := range storedCrons {
			s.hydrateProfileName(&storedCron, workspace)
			if ownerWS, ok := seen[storedCron.Config.CronID]; ok && ownerWS != workspace {
				return nil, fmt.Errorf("cron %s exists in multiple workspaces: %s and %s", storedCron.Config.CronID, ownerWS, workspace)
			}
			seen[storedCron.Config.CronID] = workspace
			entries = append(entries, storedCronEntry{workspace: workspace, stored: storedCron})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].stored.Config.CronID == entries[j].stored.Config.CronID {
			return entries[i].stored.Config.ProfileName < entries[j].stored.Config.ProfileName
		}
		return entries[i].stored.Config.CronID < entries[j].stored.Config.CronID
	})
	return entries, nil
}

func (s *cronService) findCronOwner(cronID string) (string, *StoredCron, error) {
	cronID = strings.TrimSpace(cronID)
	if cronID == "" {
		return "", nil, fmt.Errorf("cron id is required")
	}
	var ownerWorkspace string
	var stored *StoredCron
	for _, workspace := range s.resolver.Workspaces() {
		candidate, err := readCronFiles(workspace, cronID)
		if isCronNotFound(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		s.hydrateProfileName(candidate, workspace)
		if ownerWorkspace != "" && ownerWorkspace != workspace {
			return "", nil, fmt.Errorf("cron %s exists in multiple workspaces: %s and %s", cronID, ownerWorkspace, workspace)
		}
		ownerWorkspace = workspace
		stored = candidate
	}
	if ownerWorkspace == "" {
		return "", nil, fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
	}
	return ownerWorkspace, stored, nil
}

func (s *cronService) hydrateProfileName(storedCron *StoredCron, workspace string) {
	if storedCron == nil || strings.TrimSpace(storedCron.Config.ProfileName) != "" {
		return
	}
	storedCron.Config.ProfileName = s.resolver.DefaultProfileForWorkspace(workspace)
}

// --- Workspace-level file operations (pure functions) ---

func ensureCronRoot(workspace string) error {
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("workspace path is required")
	}
	return os.MkdirAll(filepath.Join(workspace, cronsDirName), 0755)
}

func listWorkspaceCrons(workspace string) ([]StoredCron, error) {
	if err := ensureCronRoot(workspace); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(workspace, cronsDirName))
	if err != nil {
		return nil, err
	}
	storedCrons := make([]StoredCron, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		storedCron, err := readCronFiles(workspace, entry.Name())
		if err != nil {
			return nil, err
		}
		storedCrons = append(storedCrons, *storedCron)
	}
	sort.Slice(storedCrons, func(i, j int) bool {
		return storedCrons[i].Config.CronID < storedCrons[j].Config.CronID
	})
	return storedCrons, nil
}

func readCronFiles(workspace string, cronID string) (*StoredCron, error) {
	cronID = strings.TrimSpace(cronID)
	if err := validateCronID(cronID); err != nil {
		return nil, err
	}
	dir, err := resolveCronDir(workspace, cronID)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, configFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	if err := validateCronID(cfg.CronID); err != nil {
		return nil, err
	}
	taskContent, err := os.ReadFile(filepath.Join(dir, taskFileName))
	if err != nil {
		return nil, err
	}
	return &StoredCron{Config: cfg, Task: strings.TrimSpace(string(taskContent)), Path: dir}, nil
}

func writeCronFiles(workspace string, storedCron StoredCron) error {
	if err := ensureCronRoot(workspace); err != nil {
		return err
	}
	if err := os.MkdirAll(storedCron.Path, 0755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(storedCron.Path, configFileName), storedCron.Config); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storedCron.Path, taskFileName), []byte(strings.TrimSpace(storedCron.Task)+"\n"), 0644)
}

func normalizeCronInput(workspace string, input UpsertCronInput) (*StoredCron, error) {
	input.CronID = strings.TrimSpace(input.CronID)
	input.CronExpression = strings.TrimSpace(input.CronExpression)
	input.Task = strings.TrimSpace(input.Task)
	if err := validateCronID(input.CronID); err != nil {
		return nil, err
	}
	if input.CronExpression == "" {
		return nil, fmt.Errorf("cron expression is required")
	}
	if _, err := ccron.ParseStandard(input.CronExpression); err != nil {
		return nil, err
	}
	if input.Task == "" {
		return nil, fmt.Errorf("task is required")
	}
	dir, err := resolveCronDir(workspace, input.CronID)
	if err != nil {
		return nil, err
	}
	return &StoredCron{
		Config: Config{
			CronID:         input.CronID,
			CronExpression: input.CronExpression,
			Enabled:        input.Enabled,
			ProfileName:    strings.TrimSpace(input.ProfileName),
			InvocationMode: strings.TrimSpace(input.InvocationMode),
		},
		Task: input.Task,
		Path: dir,
	}, nil
}

func resolveCronDir(workspace string, cronID string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	if err := validateCronID(cronID); err != nil {
		return "", err
	}
	root := filepath.Join(workspace, cronsDirName)
	candidate := filepath.Join(root, cronID)
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolvedCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cron path escapes workspace: %s", cronID)
	}
	return resolvedCandidate, nil
}

// --- Shared helpers ---

func validateCronID(cronID string) error {
	if strings.TrimSpace(cronID) == "" {
		return fmt.Errorf("cron id is required")
	}
	if !cronIDPattern.MatchString(cronID) {
		return fmt.Errorf("invalid cron id: %s", cronID)
	}
	return nil
}

func buildCronSessionID(cronID string, executionID string) string {
	return defaultCronChannelID + ":" + cronID + ":" + executionID
}

func buildExecutionPrompt(storedCron *StoredCron, executionDir string) string {
	return fmt.Sprintf("Execute cron task %q.\n\nExecution directory: %s\nStore any generated artifacts under this directory.\n\nTask definition:\n%s", storedCron.Config.CronID, filepath.ToSlash(executionDir), storedCron.Task)
}

func collectArtifacts(executionDir string) ([]string, error) {
	artifacts := []string{}
	err := filepath.WalkDir(executionDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == manifestFileName || base == sessionRefFileName {
			return nil
		}
		rel, err := filepath.Rel(executionDir, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(artifacts)
	return artifacts, nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0644)
}

func isCronNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrCronNotFound.Error())
}
