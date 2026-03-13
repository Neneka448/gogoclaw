package cron

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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

type workspaceService struct {
	workspace string
	manager   CronManager
	executor  Executor
	location  *time.Location
}

type workspaceCron struct {
	config  Config
	execute func() error
}

func NewCronService(workspace string, manager CronManager, executor Executor, location *time.Location) Service {
	if location == nil {
		location = time.Local
	}
	return &workspaceService{
		workspace: strings.TrimSpace(workspace),
		manager:   manager,
		executor:  executor,
		location:  location,
	}
}

func (cronTask *workspaceCron) Execute() error {
	return cronTask.execute()
}

func (cronTask *workspaceCron) GetCronConfig() *Config {
	config := cronTask.config
	return &config
}

func (service *workspaceService) EnsureRoot() error {
	if strings.TrimSpace(service.workspace) == "" {
		return fmt.Errorf("workspace path is required")
	}
	return os.MkdirAll(filepath.Join(service.workspace, cronsDirName), 0755)
}

func (service *workspaceService) LoadAll() error {
	storedCrons, err := service.ListCrons()
	if err != nil {
		return err
	}
	if service.manager == nil {
		return nil
	}
	for _, storedCron := range storedCrons {
		if !storedCron.Config.Enabled {
			continue
		}
		if err := service.manager.RegisterCron(service.buildRuntimeCron(storedCron)); err != nil {
			return err
		}
	}
	return nil
}

func (service *workspaceService) Start() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Start()
}

func (service *workspaceService) Stop() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Stop()
}

func (service *workspaceService) ListCrons() ([]StoredCron, error) {
	if err := service.EnsureRoot(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(service.workspace, cronsDirName))
	if err != nil {
		return nil, err
	}
	storedCrons := make([]StoredCron, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		storedCron, err := service.readCron(entry.Name())
		if err != nil {
			return nil, err
		}
		storedCrons = append(storedCrons, *storedCron)
	}
	sort.Slice(storedCrons, func(i int, j int) bool {
		return storedCrons[i].Config.CronID < storedCrons[j].Config.CronID
	})
	return storedCrons, nil
}

func (service *workspaceService) GetCron(cronID string) (*StoredCron, error) {
	return service.readCron(cronID)
}

func (service *workspaceService) CreateCron(input UpsertCronInput) (*StoredCron, error) {
	storedCron, err := service.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	cronDir, err := service.cronDir(storedCron.Config.CronID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cronDir); err == nil {
		return nil, fmt.Errorf("cron already exists: %s", storedCron.Config.CronID)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := service.writeCron(*storedCron); err != nil {
		return nil, err
	}
	if err := service.syncRuntime(*storedCron); err != nil {
		return nil, err
	}
	return service.readCron(storedCron.Config.CronID)
}

func (service *workspaceService) UpdateCron(input UpsertCronInput) (*StoredCron, error) {
	storedCron, err := service.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	if _, err := service.readCron(storedCron.Config.CronID); err != nil {
		return nil, err
	}
	if err := service.writeCron(*storedCron); err != nil {
		return nil, err
	}
	if err := service.syncRuntime(*storedCron); err != nil {
		return nil, err
	}
	return service.readCron(storedCron.Config.CronID)
}

func (service *workspaceService) DeleteCron(cronID string) error {
	cronID = strings.TrimSpace(cronID)
	if err := validateCronID(cronID); err != nil {
		return err
	}
	if service.manager != nil {
		if err := service.manager.DeleteCron(cronID); err != nil && !isCronNotFound(err) {
			return err
		}
	}
	cronDir, err := service.cronDir(cronID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(cronDir); err != nil {
		return err
	}
	return nil
}

func (service *workspaceService) ExecuteCron(cronID string) error {
	storedCron, err := service.readCron(cronID)
	if err != nil {
		return err
	}
	if !storedCron.Config.Enabled {
		return fmt.Errorf("cron is disabled: %s", cronID)
	}
	if service.executor == nil {
		return fmt.Errorf("cron executor is not configured")
	}

	startedAt := service.currentTime()
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

	execErr := service.executor(ExecutionRequest{
		CronID:       storedCron.Config.CronID,
		SessionID:    sessionID,
		Prompt:       buildExecutionPrompt(storedCron, executionDir),
		ExecutionDir: executionDir,
		Metadata: map[string]string{
			"source":  defaultCronChannelID,
			"cron_id": storedCron.Config.CronID,
			"exec_id": executionID,
		},
	})

	manifest.Status = "succeeded"
	if execErr != nil {
		manifest.Status = "failed"
		manifest.Error = execErr.Error()
	}
	manifest.FinishedAt = service.currentTime().Format(time.RFC3339)
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

func (service *workspaceService) buildRuntimeCron(storedCron StoredCron) Cron {
	config := storedCron.Config
	return &workspaceCron{
		config: config,
		execute: func() error {
			return service.ExecuteCron(config.CronID)
		},
	}
}

func (service *workspaceService) currentTime() time.Time {
	now := cronNow()
	if service.location == nil {
		return now
	}
	return now.In(service.location)
}

func (service *workspaceService) syncRuntime(storedCron StoredCron) error {
	if service.manager == nil {
		return nil
	}
	if err := service.manager.RegisterCron(service.buildRuntimeCron(storedCron)); err != nil {
		return err
	}
	if !storedCron.Config.Enabled {
		return service.manager.DeleteCron(storedCron.Config.CronID)
	}
	return nil
}

func (service *workspaceService) normalizeInput(input UpsertCronInput) (*StoredCron, error) {
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
	cronDir, err := service.cronDir(input.CronID)
	if err != nil {
		return nil, err
	}
	return &StoredCron{
		Config: Config{
			CronID:         input.CronID,
			CronExpression: input.CronExpression,
			Enabled:        input.Enabled,
		},
		Task: input.Task,
		Path: cronDir,
	}, nil
}

func (service *workspaceService) readCron(cronID string) (*StoredCron, error) {
	cronID = strings.TrimSpace(cronID)
	if err := validateCronID(cronID); err != nil {
		return nil, err
	}
	cronDir, err := service.cronDir(cronID)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(cronDir, configFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
		}
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}
	if err := validateCronID(config.CronID); err != nil {
		return nil, err
	}
	taskContent, err := os.ReadFile(filepath.Join(cronDir, taskFileName))
	if err != nil {
		return nil, err
	}
	return &StoredCron{Config: config, Task: strings.TrimSpace(string(taskContent)), Path: cronDir}, nil
}

func (service *workspaceService) writeCron(storedCron StoredCron) error {
	if err := service.EnsureRoot(); err != nil {
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

func (service *workspaceService) cronDir(cronID string) (string, error) {
	if strings.TrimSpace(service.workspace) == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	if err := validateCronID(cronID); err != nil {
		return "", err
	}
	root := filepath.Join(service.workspace, cronsDirName)
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
	relExecutionDir := executionDir
	return fmt.Sprintf("Execute cron task %q.\n\nExecution directory: %s\nStore any generated artifacts under this directory.\n\nTask definition:\n%s", storedCron.Config.CronID, filepath.ToSlash(relExecutionDir), storedCron.Task)
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
