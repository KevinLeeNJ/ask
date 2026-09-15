package managedblock

import (
	"fmt"
	"strings"
)

const (
	StartMarker = "# >>> ask managed secrets >>>"
	EndMarker   = "# <<< ask managed secrets <<<"
)

func Upsert(content, variable, line string, adapterVariable func(string) string) (string, bool, error) {
	before, block, after, found, err := split(content)
	if err != nil {
		return "", false, err
	}
	kept := make([]string, 0, len(block)+1)
	replaced := false
	for _, existing := range block {
		if adapterVariable(existing) == variable {
			if !replaced {
				kept = append(kept, line)
				replaced = true
			}
			continue
		}
		kept = append(kept, existing)
	}
	if !replaced {
		kept = append(kept, line)
	}
	return assemble(before, kept, after), found, nil
}

func Remove(content, variable string, adapterVariable func(string) string) (string, error) {
	before, block, after, _, err := split(content)
	if err != nil {
		return "", err
	}
	kept := make([]string, 0, len(block))
	for _, existing := range block {
		if adapterVariable(existing) == variable {
			continue
		}
		kept = append(kept, existing)
	}
	return assemble(before, kept, after), nil
}

func split(content string) (string, []string, string, bool, error) {
	startCount := strings.Count(content, StartMarker)
	endCount := strings.Count(content, EndMarker)
	if startCount != endCount || startCount > 1 {
		return "", nil, "", false, fmt.Errorf("ask managed block markers are incomplete or duplicated")
	}
	if startCount == 0 {
		return strings.TrimRight(content, "\n"), nil, "", false, nil
	}
	start := strings.Index(content, StartMarker)
	end := strings.Index(content, EndMarker)
	if end < start {
		return "", nil, "", false, fmt.Errorf("ask managed block markers are out of order")
	}
	before := content[:start]
	after := content[end+len(EndMarker):]
	blockText := content[start+len(StartMarker) : end]
	lines := nonEmptyLines(blockText)
	return strings.TrimRight(before, "\n"), lines, strings.TrimLeft(after, "\n"), true, nil
}

func assemble(before string, block []string, after string) string {
	var builder strings.Builder
	if strings.TrimSpace(before) != "" {
		builder.WriteString(strings.TrimRight(before, "\n"))
		builder.WriteString("\n\n")
	}
	if len(block) > 0 {
		builder.WriteString(StartMarker)
		builder.WriteByte('\n')
		for _, line := range block {
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
		builder.WriteString(EndMarker)
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(after) != "" {
		builder.WriteString(strings.TrimLeft(after, "\n"))
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func nonEmptyLines(value string) []string {
	raw := strings.Split(value, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
