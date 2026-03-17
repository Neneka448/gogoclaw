package cron

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type multiProfileService struct {
	defaultProfile      string
	manager             CronManager
	executor            Executor
	location            *time.Location
	profileToWorkspace  map[string]string
	workspaceToProfiles map[string][]string
	workspaceServices   map[string]*workspaceService
	workspaceOrder      []string
}

type storedCronEntry struct {
	service *workspaceService
	stored  StoredCron
}

func NewMultiProfileService(profileWorkspaces map[string]string, defaultProfile string, manager CronManager, executor Executor, location *time.Location) Service {
	if location == nil {
		location = time.Local
	}
	resolvedDefault := strings.TrimSpace(defaultProfile)
	if resolvedDefault == "" {
		resolvedDefault = "default"
	}

	service := &multiProfileService{
		defaultProfile:      resolvedDefault,
		manager:             manager,
		executor:            executor,
		location:            location,
		profileToWorkspace:  make(map[string]string),
		workspaceToProfiles: make(map[string][]string),
		workspaceServices:   make(map[string]*workspaceService),
	}

	profiles := make([]string, 0, len(profileWorkspaces))
	for profileName := range profileWorkspaces {
		trimmed := strings.TrimSpace(profileName)
		if trimmed == "" {
			continue
		}
		profiles = append(profiles, trimmed)
	}
	sort.Strings(profiles)

	for _, profileName := range profiles {
		workspace := canonicalWorkspace(profileWorkspaces[profileName])
		service.profileToWorkspace[profileName] = workspace
		service.workspaceToProfiles[workspace] = append(service.workspaceToProfiles[workspace], profileName)
		if _, ok := service.workspaceServices[workspace]; !ok {
			child, _ := NewCronService(workspace, manager, executor, location).(*workspaceService)
			service.workspaceServices[workspace] = child
			service.workspaceOrder = append(service.workspaceOrder, workspace)
		}
	}
	sort.Strings(service.workspaceOrder)
	for workspace := range service.workspaceToProfiles {
		sort.Strings(service.workspaceToProfiles[workspace])
	}

	return service
}

func (service *multiProfileService) EnsureRoot() error {
	for _, workspace := range service.workspaceOrder {
		child := service.workspaceServices[workspace]
		if child == nil {
			continue
		}
		if err := child.EnsureRoot(); err != nil {
			return err
		}
	}
	return nil
}

func (service *multiProfileService) LoadAll() error {
	entries, err := service.listEntries()
	if err != nil {
		return err
	}
	if service.manager == nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.stored.Config.Enabled {
			continue
		}
		if err := service.manager.RegisterCron(entry.service.buildRuntimeCron(entry.stored)); err != nil {
			return err
		}
	}
	return nil
}

func (service *multiProfileService) Start() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Start()
}

func (service *multiProfileService) Stop() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Stop()
}

func (service *multiProfileService) ListCrons() ([]StoredCron, error) {
	entries, err := service.listEntries()
	if err != nil {
		return nil, err
	}
	crons := make([]StoredCron, 0, len(entries))
	for _, entry := range entries {
		crons = append(crons, entry.stored)
	}
	return crons, nil
}

func (service *multiProfileService) GetCron(cronID string) (*StoredCron, error) {
	_, stored, err := service.findOwner(cronID)
	return stored, err
}

func (service *multiProfileService) CreateCron(input UpsertCronInput) (*StoredCron, error) {
	targetProfile, child, err := service.serviceForProfile(input.ProfileName)
	if err != nil {
		return nil, err
	}
	if owner, existing, err := service.findOwner(strings.TrimSpace(input.CronID)); err == nil {
		return nil, fmt.Errorf("cron already exists in workspace %s for profile %s: %s", owner.workspace, existing.Config.ProfileName, existing.Config.CronID)
	} else if !isCronNotFound(err) {
		return nil, err
	}
	input.ProfileName = targetProfile
	return child.CreateCron(input)
}

func (service *multiProfileService) UpdateCron(input UpsertCronInput) (*StoredCron, error) {
	targetProfile, child, err := service.serviceForProfile(input.ProfileName)
	if err != nil {
		return nil, err
	}
	owner, _, ownerErr := service.findOwner(strings.TrimSpace(input.CronID))
	if ownerErr != nil && !isCronNotFound(ownerErr) {
		return nil, ownerErr
	}
	if owner != nil && owner.workspace != child.workspace {
		return nil, fmt.Errorf("cron %s belongs to workspace %s and cannot be moved to %s", strings.TrimSpace(input.CronID), owner.workspace, child.workspace)
	}
	input.ProfileName = targetProfile
	return child.UpdateCron(input)
}

func (service *multiProfileService) DeleteCron(cronID string) error {
	owner, _, err := service.findOwner(cronID)
	if err != nil {
		return err
	}
	return owner.DeleteCron(cronID)
}

func (service *multiProfileService) ExecuteCron(cronID string) error {
	owner, _, err := service.findOwner(cronID)
	if err != nil {
		return err
	}
	return owner.ExecuteCron(cronID)
}

func (service *multiProfileService) listEntries() ([]storedCronEntry, error) {
	entries := make([]storedCronEntry, 0)
	seen := make(map[string]string)
	for _, workspace := range service.workspaceOrder {
		child := service.workspaceServices[workspace]
		if child == nil {
			continue
		}
		storedCrons, err := child.ListCrons()
		if err != nil {
			return nil, err
		}
		for _, storedCron := range storedCrons {
			service.fillMissingProfileName(&storedCron, workspace)
			if ownerWorkspace, ok := seen[storedCron.Config.CronID]; ok && ownerWorkspace != workspace {
				return nil, fmt.Errorf("cron %s exists in multiple workspaces: %s and %s", storedCron.Config.CronID, ownerWorkspace, workspace)
			}
			seen[storedCron.Config.CronID] = workspace
			entries = append(entries, storedCronEntry{service: child, stored: storedCron})
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

func (service *multiProfileService) findOwner(cronID string) (*workspaceService, *StoredCron, error) {
	cronID = strings.TrimSpace(cronID)
	if cronID == "" {
		return nil, nil, fmt.Errorf("cron id is required")
	}
	var owner *workspaceService
	var stored *StoredCron
	for _, workspace := range service.workspaceOrder {
		child := service.workspaceServices[workspace]
		if child == nil {
			continue
		}
		candidate, err := child.readCron(cronID)
		if isCronNotFound(err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		service.fillMissingProfileName(candidate, workspace)
		if owner != nil && owner.workspace != child.workspace {
			return nil, nil, fmt.Errorf("cron %s exists in multiple workspaces: %s and %s", cronID, owner.workspace, child.workspace)
		}
		owner = child
		stored = candidate
	}
	if owner == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
	}
	return owner, stored, nil
}

func (service *multiProfileService) serviceForProfile(profileName string) (string, *workspaceService, error) {
	resolvedProfile := service.normalizeProfileName(profileName)
	workspace, ok := service.profileToWorkspace[resolvedProfile]
	if !ok {
		return "", nil, fmt.Errorf("profile not found: %s", resolvedProfile)
	}
	child := service.workspaceServices[workspace]
	if child == nil {
		return "", nil, fmt.Errorf("workspace service not found for profile %s", resolvedProfile)
	}
	return resolvedProfile, child, nil
}

func (service *multiProfileService) normalizeProfileName(profileName string) string {
	trimmed := strings.TrimSpace(profileName)
	if trimmed == "" {
		return service.defaultProfile
	}
	return trimmed
}

func (service *multiProfileService) fillMissingProfileName(storedCron *StoredCron, workspace string) {
	if storedCron == nil || strings.TrimSpace(storedCron.Config.ProfileName) != "" {
		return
	}
	profiles := service.workspaceToProfiles[workspace]
	if len(profiles) == 1 {
		storedCron.Config.ProfileName = profiles[0]
		return
	}
	for _, profileName := range profiles {
		if profileName == service.defaultProfile {
			storedCron.Config.ProfileName = service.defaultProfile
			return
		}
	}
}

func canonicalWorkspace(workspace string) string {
	trimmed := strings.TrimSpace(workspace)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}
