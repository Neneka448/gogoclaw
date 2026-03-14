package workspace

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	agentsFile    = "AGENTS.md"
	soulFile      = "SOUL.md"
	toolsFile     = "TOOLS.md"
	userFile      = "USER.md"
	heartbeatFile = "HEARTBEAT.md"
	skillFileName = "SKILL.md"
)

var bootstrapFileNames = []string{agentsFile, soulFile, toolsFile, userFile, heartbeatFile}

//go:embed templates/*.md templates/skills/*/SKILL.md
var bootstrapTemplates embed.FS

func BootstrapFileNames() []string {
	files := make([]string, len(bootstrapFileNames))
	copy(files, bootstrapFileNames)
	return files
}

func EnsureBootstrapFiles(workspacePath string) error {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}

	for _, fileName := range bootstrapFileNames {
		targetPath := filepath.Join(workspacePath, fileName)
		if _, err := os.Stat(targetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat bootstrap file %s: %w", targetPath, err)
		}

		content, err := bootstrapTemplates.ReadFile(filepath.Join("templates", fileName))
		if err != nil {
			return fmt.Errorf("read embedded bootstrap template %s: %w", fileName, err)
		}
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("write bootstrap file %s: %w", targetPath, err)
		}
	}

	return nil
}

// EnsureDefaultSkills deploys embedded default skill templates into the workspace
// skills directory. Existing skills are preserved and not overwritten.
func EnsureDefaultSkills(workspacePath string) error {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}

	skillEntries, err := bootstrapTemplates.ReadDir("templates/skills")
	if err != nil {
		return fmt.Errorf("read embedded skill templates: %w", err)
	}

	for _, entry := range skillEntries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		targetDir := filepath.Join(workspacePath, "skills", skillName)
		targetPath := filepath.Join(targetDir, skillFileName)

		if _, err := os.Stat(targetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat skill file %s: %w", targetPath, err)
		}

		content, err := bootstrapTemplates.ReadFile(filepath.Join("templates", "skills", skillName, skillFileName))
		if err != nil {
			return fmt.Errorf("read embedded skill template %s: %w", skillName, err)
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("create skill directory %s: %w", targetDir, err)
		}
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("write skill file %s: %w", targetPath, err)
		}
	}

	return nil
}
