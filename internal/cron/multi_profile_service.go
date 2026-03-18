package cron

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
)

type multiProfileService struct {
	resolver          *config.ProfileResolver
	manager           CronManager
	executor          Executor
	location          *time.Location
	workspaceServices map[string]*workspaceService
}

type storedCronEntry struct {
	service *workspaceService
	stored  StoredCron
}

func NewMultiProfileService(resolver *config.ProfileResolver, manager CronManager, executor Executor, location *time.Location) Service {
	if location == nil {
		location = time.Local
	}

	service := &multiProfileService{
		resolver:          resolver,
		manager:           manager,
		executor:          executor,
		location:          location,
		workspaceServices: make(map[string]*workspaceService),
	}

	for _, workspace := range resolver.Workspaces() {
		child, _ := NewCronService(workspace, manager, executor, location).(*workspaceService)
		service.workspaceServices[workspace] = child
	}

	return service
}

func (service *multiProfileService) EnsureRoot() error {
	for _, workspace := range service.resolver.Workspaces() {
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
	resolvedProfile, workspace, err := service.resolver.ResolveWorkspace(input.ProfileName)
	if err != nil {
		return nil, err
	}
	child := service.workspaceServices[workspace]
	if child == nil {
		return nil, fmt.Errorf("workspace service not found for profile %s", resolvedProfile)
	}
	if owner, existing, findErr := service.findOwner(strings.TrimSpace(input.CronID)); findErr == nil {
		return nil, fmt.Errorf("cron already exists in workspace %s for profile %s: %s", owner.workspace, existing.Config.ProfileName, existing.Config.CronID)
	} else if !isCronNotFound(findErr) {
		return nil, findErr
	}
	input.ProfileName = resolvedProfile
	return child.CreateCron(input)
}

func (service *multiProfileService) UpdateCron(input UpsertCronInput) (*StoredCron, error) {
	resolvedProfile, workspace, err := service.resolver.ResolveWorkspace(input.ProfileName)
	if err != nil {
		return nil, err
	}
	child := service.workspaceServices[workspace]
	if child == nil {
		return nil, fmt.Errorf("workspace service not found for profile %s", resolvedProfile)
	}
	owner, _, ownerErr := service.findOwner(strings.TrimSpace(input.CronID))
	if ownerErr != nil && !isCronNotFound(ownerErr) {
		return nil, ownerErr
	}
	if owner != nil && owner.workspace != child.workspace {
		return nil, fmt.Errorf("cron %s belongs to workspace %s and cannot be moved to %s", strings.TrimSpace(input.CronID), owner.workspace, child.workspace)
	}
	input.ProfileName = resolvedProfile
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
	owner, stored, err := service.findOwner(cronID)
	if err != nil {
		return err
	}
	return owner.executeStoredCron(*stored)
}

func (service *multiProfileService) listEntries() ([]storedCronEntry, error) {
	entries := make([]storedCronEntry, 0)
	seen := make(map[string]string)
	for _, workspace := range service.resolver.Workspaces() {
		child := service.workspaceServices[workspace]
		if child == nil {
			continue
		}
		storedCrons, err := child.ListCrons()
		if err != nil {
			return nil, err
		}
		for _, storedCron := range storedCrons {
			service.hydrateProfileName(&storedCron, workspace)
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
	for _, workspace := range service.resolver.Workspaces() {
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
		service.hydrateProfileName(candidate, workspace)
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

// hydrateProfileName fills in a missing profile name on a stored cron
// by delegating to the profile resolver. This is only used for display
// and sorting; it does not change persisted state.
func (service *multiProfileService) hydrateProfileName(storedCron *StoredCron, workspace string) {
	if storedCron == nil || strings.TrimSpace(storedCron.Config.ProfileName) != "" {
		return
	}
	storedCron.Config.ProfileName = service.resolver.DefaultProfileForWorkspace(workspace)
}
