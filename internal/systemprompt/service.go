package systemprompt

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Neneka448/gogoclaw/internal/skills"
)

type Service interface {
	Build(skillRegistry skills.Registry) string
}

type service struct {
	workspace string

	loadOnce sync.Once
	sections promptSections
	loadErr  error
}

type promptSections struct {
	Agents string
	Soul   string
	Tools  string
	User   string
}

func NewService(workspace string) Service {
	return &service{workspace: strings.TrimSpace(workspace)}
}

func (service *service) Build(skillRegistry skills.Registry) string {
	service.loadOnce.Do(func() {
		service.sections, service.loadErr = loadPromptSections(service.workspace)
	})

	parts := make([]string, 0, 5)
	if service.sections.Agents != "" {
		parts = append(parts, xmlSection("agents", service.sections.Agents))
	}
	if service.sections.Soul != "" {
		parts = append(parts, xmlSection("soul", service.sections.Soul))
	}
	if service.sections.Tools != "" {
		parts = append(parts, xmlSection("tools", service.sections.Tools))
	}
	if service.sections.User != "" {
		parts = append(parts, xmlSection("user", service.sections.User))
	}

	skillsSection := buildSkillsSection(skillRegistry)
	if skillsSection != "" {
		parts = append(parts, xmlSection("skills", skillsSection))
	}
	if service.loadErr != nil {
		parts = append(parts, xmlSection("system_prompt_error", service.loadErr.Error()))
	}
	if len(parts) == 0 {
		return ""
	}

	return "<system_prompt>\n" + strings.Join(parts, "\n\n") + "\n</system_prompt>"
}

func loadPromptSections(workspace string) (promptSections, error) {
	if strings.TrimSpace(workspace) == "" {
		return promptSections{}, nil
	}

	sections := promptSections{}
	var errors []string
	for _, file := range []struct {
		name string
		set  func(string)
	}{
		{name: "AGENTS.md", set: func(content string) { sections.Agents = content }},
		{name: "SOUL.md", set: func(content string) { sections.Soul = content }},
		{name: "TOOLS.md", set: func(content string) { sections.Tools = content }},
		{name: "USER.md", set: func(content string) { sections.User = content }},
	} {
		content, err := os.ReadFile(filepath.Join(workspace, file.name))
		if err != nil {
			if !os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("read %s: %v", file.name, err))
			}
			continue
		}
		file.set(strings.TrimSpace(string(content)))
	}

	if len(errors) == 0 {
		return sections, nil
	}
	return sections, fmt.Errorf("%s", strings.Join(errors, "; "))
}

func buildSkillsSection(skillRegistry skills.Registry) string {
	if skillRegistry == nil || skillRegistry.Len() == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("You have access to reusable workspace skills. Each skill lives under workspace/skills/<skill-name>/SKILL.md.\n")
	builder.WriteString("Before solving the current request, compare the request against the available skill metadata below and judge whether the scenario matches a skill.\n")
	builder.WriteString("If a skill appears relevant, call get_skill with the skill name before continuing so you can read the full SKILL.md instructions.\n")
	builder.WriteString("If no skill matches, continue normally without calling get_skill.\n")
	builder.WriteString("Available skills:\n")

	for _, skill := range skillRegistry.GetAll() {
		metadata := "{}"
		if len(skill.FrontMatter) > 0 {
			encoded, err := json.Marshal(skill.FrontMatter)
			if err == nil {
				metadata = string(encoded)
			}
		}
		builder.WriteString(fmt.Sprintf("- %s: %s\n", skill.Name, metadata))
	}

	return strings.TrimSpace(builder.String())
}

func xmlSection(name string, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	return "<" + name + ">\n" + escapeXML(trimmed) + "\n</" + name + ">"
}

func escapeXML(content string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(content)); err != nil {
		return content
	}
	return buffer.String()
}
