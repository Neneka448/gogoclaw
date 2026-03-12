package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

type Skill struct {
	Name        string
	Directory   string
	FilePath    string
	FrontMatter map[string]any
	Content     string
}

type Registry interface {
	Register(skill Skill) error
	Get(name string) (Skill, error)
	GetAll() []Skill
	Len() int
}

type registry struct {
	skills map[string]Skill
}

func NewRegistry() Registry {
	return &registry{skills: make(map[string]Skill)}
}

func LoadWorkspaceSkills(workspace string) (Registry, error) {
	registry := NewRegistry()
	if strings.TrimSpace(workspace) == "" {
		return registry, nil
	}

	skillsRoot := filepath.Join(workspace, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return nil, fmt.Errorf("read skills directory %s: %w", skillsRoot, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		filePath := filepath.Join(skillsRoot, skillName, skillFileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("skill %s is missing %s", skillName, skillFileName)
			}
			return nil, fmt.Errorf("read skill file %s: %w", filePath, err)
		}

		frontMatter, err := parseFrontMatter(content)
		if err != nil {
			return nil, fmt.Errorf("load skill %s: %w", skillName, err)
		}

		if err := registry.Register(Skill{
			Name:        skillName,
			Directory:   filepath.Join(skillsRoot, skillName),
			FilePath:    filePath,
			FrontMatter: frontMatter,
			Content:     string(content),
		}); err != nil {
			return nil, err
		}
	}

	return registry, nil
}

func (registry *registry) Register(skill Skill) error {
	if strings.TrimSpace(skill.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if _, exists := registry.skills[skill.Name]; exists {
		return fmt.Errorf("skill already registered: %s", skill.Name)
	}
	if skill.FrontMatter == nil {
		skill.FrontMatter = map[string]any{}
	}
	registry.skills[skill.Name] = skill
	return nil
}

func (registry *registry) Get(name string) (Skill, error) {
	skill, ok := registry.skills[name]
	if !ok {
		return Skill{}, fmt.Errorf("skill not found: %s", name)
	}
	return skill, nil
}

func (registry *registry) GetAll() []Skill {
	all := make([]Skill, 0, len(registry.skills))
	for _, skill := range registry.skills {
		all = append(all, skill)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})
	return all
}

func (registry *registry) Len() int {
	return len(registry.skills)
}

func parseFrontMatter(content []byte) (map[string]any, error) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") && text != "---" && !strings.HasPrefix(text, "---\r\n") {
		return map[string]any{}, nil
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]any{}, nil
	}

	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end == -1 {
		return nil, fmt.Errorf("skill front matter is missing closing ---")
	}

	body := strings.Join(lines[1:end], "\n")
	if strings.TrimSpace(body) == "" {
		return map[string]any{}, nil
	}

	metadata := map[string]any{}
	if err := yaml.Unmarshal([]byte(body), &metadata); err != nil {
		return nil, fmt.Errorf("parse skill front matter: %w", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}

	return metadata, nil
}
