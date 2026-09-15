package title

import (
	"strings"
	"unicode/utf8"
)

const (
	provisionalLimit = 30
	requestLimit     = 2000
)

func Provisional(question string) string {
	normalized := collapse(question)
	return truncate(normalized, provisionalLimit)
}

func NormalizeGenerated(value string) string {
	line := ""
	for _, candidate := range strings.Split(value, "\n") {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			line = candidate
			break
		}
	}
	if line == "" {
		return ""
	}
	for _, prefix := range []string{"标题:", "标题：", "Title:", "title:"} {
		line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
	}
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "`'\"“”‘’*_#")
	line = collapse(line)
	line = strings.TrimRight(line, ".!?;:。！？；：")
	line = strings.TrimSpace(strings.Trim(line, "`'\"“”‘’*_#"))
	return truncate(strings.TrimSpace(line), provisionalLimit)
}

func RequestText(question string) string {
	return truncate(collapse(question), requestLimit)
}

func SystemPrompt() string {
	return "Generate a concise plain-text title for the user's first question. " +
		"Use the question's primary language. Output only the title, no Markdown, quotes, prefix, explanation, sentence punctuation, or newline."
}

func collapse(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

func truncate(input string, limit int) string {
	if utf8.RuneCountInString(input) <= limit {
		return input
	}
	runes := []rune(input)
	return string(runes[:limit]) + "..."
}
