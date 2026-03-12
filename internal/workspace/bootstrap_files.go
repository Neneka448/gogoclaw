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
)

var bootstrapFileNames = []string{agentsFile, soulFile, toolsFile, userFile, heartbeatFile}

//go:embed templates/*.md
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
