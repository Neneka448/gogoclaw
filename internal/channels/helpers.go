package channels

import (
	"sort"
	"strings"
)

func firstString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		current := any(payload)
		matched := true
		for _, part := range strings.Split(path, ".") {
			next, ok := current.(map[string]any)
			if !ok {
				matched = false
				break
			}
			current, ok = next[part]
			if !ok {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		value, ok := current.(string)
		if ok {
			return value
		}
	}
	return ""
}

func getAnySliceByPath(payload map[string]any, path string) []any {
	current := any(payload)
	for _, part := range strings.Split(path, ".") {
		next, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = next[part]
		if !ok {
			return nil
		}
	}
	values, ok := current.([]any)
	if !ok {
		return nil
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
