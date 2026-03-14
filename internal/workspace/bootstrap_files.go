package workspace

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	agentsFile       = "AGENTS.md"
	soulFile         = "SOUL.md"
	toolsFile        = "TOOLS.md"
	userFile         = "USER.md"
	heartbeatFile    = "HEARTBEAT.md"
	memorySkillFile  = "MEMORY_SKILL.md"
	skillFileName    = "SKILL.md"
)

var bootstrapFileNames = []string{agentsFile, soulFile, toolsFile, userFile, heartbeatFile}

//go:embed templates/*.md templates/skills
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

		content, err := bootstrapTemplates.ReadFile(path.Join("templates", fileName))
		if err != nil {
			return fmt.Errorf("read embedded bootstrap template %s: %w", fileName, err)
		}
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("write bootstrap file %s: %w", targetPath, err)
		}
	}

	return nil
}

// EnsureMemorySkill creates the memory skill directory and SKILL.md if it does not exist.
func EnsureMemorySkill(workspacePath string) error {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}

	skillDir := filepath.Join(workspacePath, "skills", "memory")
	targetPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat memory skill file: %w", err)
	}

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("create memory skill directory: %w", err)
	}

	content, err := bootstrapTemplates.ReadFile(path.Join("templates", memorySkillFile))
	if err != nil {
		return fmt.Errorf("read memory skill template: %w", err)
	}
	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		return fmt.Errorf("write memory skill file: %w", err)
	}

	return nil
}

// EnsureDefaultSkills deploys embedded default skill templates into the workspace
// skills directory. If a skill's SKILL.md already exists, the entire skill is
// skipped (existing skills are preserved). New skills are deployed with all
// bundled resources (agents, scripts, references, etc.).
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

		embeddedRoot := filepath.Join("templates", "skills", skillName)
		if err := deployEmbeddedDir(bootstrapTemplates, embeddedRoot, targetDir); err != nil {
			return fmt.Errorf("deploy skill %s: %w", skillName, err)
		}
	}

	return nil
}

// deployEmbeddedDir recursively copies all files from an embedded FS directory
// to a target directory on disk.
func deployEmbeddedDir(fsys embed.FS, embeddedRoot, targetDir string) error {
	return fs.WalkDir(fsys, embeddedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(embeddedRoot, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		content, err := fsys.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded file %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", targetPath, err)
		}
		return os.WriteFile(targetPath, content, 0644)
	})
}
