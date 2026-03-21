package workspace

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	agentsFile      = "AGENTS.md"
	soulFile        = "SOUL.md"
	toolsFile       = "TOOLS.md"
	userFile        = "USER.md"
	heartbeatFile   = "HEARTBEAT.md"
	memorySkillFile = "MEMORY_SKILL.md"
	skillFileName   = "SKILL.md"
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
// skills directory. Existing SKILL.md files are preserved, while bundled
// resources such as scripts, references, assets, and templates are kept in sync
// with the embedded defaults.
func EnsureDefaultSkills(workspacePath string) error {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}

	skillEntries, err := bootstrapTemplates.ReadDir("templates/skills")
	if err != nil {
		return fmt.Errorf("read embedded skill templates: %w", err)
	}

	// Migrate legacy hyphenated skill directories to underscore naming so that
	// python -m imports work. If both old and new exist, remove the old one.
	legacyRenames := map[string]string{
		"cron-task":    "cron_task",
		"invoke-agent": "invoke_agent",
	}
	for oldName, newName := range legacyRenames {
		oldDir := filepath.Join(workspacePath, "skills", oldName)
		newDir := filepath.Join(workspacePath, "skills", newName)
		if _, err := os.Stat(oldDir); err == nil {
			if _, err := os.Stat(newDir); err == nil {
				// New dir already deployed — remove the legacy one.
				os.RemoveAll(oldDir)
			} else {
				// Rename old → new.
				os.Rename(oldDir, newDir)
			}
		}
	}

	for _, entry := range skillEntries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		targetDir := filepath.Join(workspacePath, "skills", skillName)

		embeddedRoot := filepath.Join("templates", "skills", skillName)
		if err := syncEmbeddedSkillDir(bootstrapTemplates, embeddedRoot, targetDir); err != nil {
			return fmt.Errorf("deploy skill %s: %w", skillName, err)
		}
	}

	return nil
}

// syncEmbeddedSkillDir recursively copies all files from an embedded skill
// directory to a target directory on disk. Existing SKILL.md files are
// preserved so workspace-local skill protocols are not overwritten.
func syncEmbeddedSkillDir(fsys embed.FS, embeddedRoot, targetDir string) error {
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

		mode := fs.FileMode(0644)
		if isExecutableScript(path, content) {
			mode = 0755
		}

		if filepath.Base(targetPath) == skillFileName {
			if _, err := os.Stat(targetPath); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("stat skill file %s: %w", targetPath, err)
			}
		}

		return os.WriteFile(targetPath, content, mode)
	})
}

func isExecutableScript(path string, content []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sh", ".py":
		return true
	}

	if len(content) == 0 {
		return false
	}

	// Check for shebang (#!) on the first line.
	lineEnd := bytes.IndexByte(content, '\n')
	if lineEnd == -1 {
		lineEnd = len(content)
	}
	firstLine := string(content[:lineEnd])

	return strings.HasPrefix(firstLine, "#!")
}
