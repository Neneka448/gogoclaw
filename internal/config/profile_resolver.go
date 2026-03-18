package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ProfileResolver resolves profile names to workspaces and provides
// profile-level queries. It is constructed once from the system config
// and shared across modules so that no downstream package needs to
// parse or normalize profile configuration.
type ProfileResolver struct {
	defaultProfile      string
	profileToWorkspace  map[string]string
	workspaceToProfiles map[string][]string
	workspaceOrder      []string
}

// NewProfileResolver builds a resolver from the agent profiles section
// of the system config. defaultProfile is the fallback when a caller
// supplies an empty profile name (typically "default").
func NewProfileResolver(profiles map[string]ProfileConfig, defaultProfile string) *ProfileResolver {
	resolvedDefault := strings.TrimSpace(defaultProfile)
	if resolvedDefault == "" {
		resolvedDefault = "default"
	}

	resolver := &ProfileResolver{
		defaultProfile:      resolvedDefault,
		profileToWorkspace:  make(map[string]string),
		workspaceToProfiles: make(map[string][]string),
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	sort.Strings(names)

	seen := map[string]struct{}{}
	for _, name := range names {
		workspace := canonicalWorkspacePath(profiles[name].Workspace)
		resolver.profileToWorkspace[name] = workspace
		resolver.workspaceToProfiles[workspace] = append(resolver.workspaceToProfiles[workspace], name)
		if _, ok := seen[workspace]; !ok {
			seen[workspace] = struct{}{}
			resolver.workspaceOrder = append(resolver.workspaceOrder, workspace)
		}
	}
	sort.Strings(resolver.workspaceOrder)
	for workspace := range resolver.workspaceToProfiles {
		sort.Strings(resolver.workspaceToProfiles[workspace])
	}

	return resolver
}

// NormalizeProfileName returns the trimmed profile name, falling back
// to the configured default when the input is empty.
func (r *ProfileResolver) NormalizeProfileName(profileName string) string {
	trimmed := strings.TrimSpace(profileName)
	if trimmed == "" {
		return r.defaultProfile
	}
	return trimmed
}

// DefaultProfile returns the configured default profile name.
func (r *ProfileResolver) DefaultProfile() string {
	return r.defaultProfile
}

// ResolveWorkspace maps a profile name to its resolved name and
// workspace path. An empty profileName is normalized to the default.
func (r *ProfileResolver) ResolveWorkspace(profileName string) (resolvedProfile string, workspace string, err error) {
	resolved := r.NormalizeProfileName(profileName)
	workspace, ok := r.profileToWorkspace[resolved]
	if !ok {
		return "", "", fmt.Errorf("profile not found: %s", resolved)
	}
	return resolved, workspace, nil
}

// Workspaces returns all unique workspace paths in stable order.
func (r *ProfileResolver) Workspaces() []string {
	result := make([]string, len(r.workspaceOrder))
	copy(result, r.workspaceOrder)
	return result
}

// WorkspaceProfiles returns the profile names bound to a workspace,
// in sorted order.
func (r *ProfileResolver) WorkspaceProfiles(workspace string) []string {
	profiles := r.workspaceToProfiles[workspace]
	result := make([]string, len(profiles))
	copy(result, profiles)
	return result
}

// DefaultProfileForWorkspace returns the best-effort profile name for
// a workspace that has crons with no explicit profile. If the workspace
// has exactly one profile, it returns that; otherwise it returns the
// global default profile if it belongs to that workspace, or "".
func (r *ProfileResolver) DefaultProfileForWorkspace(workspace string) string {
	profiles := r.workspaceToProfiles[workspace]
	if len(profiles) == 1 {
		return profiles[0]
	}
	for _, name := range profiles {
		if name == r.defaultProfile {
			return r.defaultProfile
		}
	}
	return ""
}

// ProfileWorkspaces returns the full profile-to-workspace map.
// The returned map is a copy and safe to mutate.
func (r *ProfileResolver) ProfileWorkspaces() map[string]string {
	result := make(map[string]string, len(r.profileToWorkspace))
	for k, v := range r.profileToWorkspace {
		result[k] = v
	}
	return result
}

func canonicalWorkspacePath(workspace string) string {
	trimmed := strings.TrimSpace(workspace)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}
