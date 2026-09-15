package locale

import (
	"os"
	"strings"
)

const (
	Chinese = "zh-CN"
	English = "en-US"
)

func Resolve(cliOverride, configured string) string {
	if value := Normalize(cliOverride); value != "" && cliOverride != "auto" {
		return value
	}
	if value := Normalize(configured); value != "" && configured != "auto" {
		return value
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := Normalize(os.Getenv(key)); value != "" {
			return value
		}
	}
	return English
}

func Normalize(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" || value == "c" || value == "posix" {
		return ""
	}
	value = strings.SplitN(value, ".", 2)[0]
	value = strings.SplitN(value, "@", 2)[0]
	value = strings.ReplaceAll(value, "_", "-")
	switch {
	case value == "zh" || strings.HasPrefix(value, "zh-"):
		return Chinese
	case value == "en" || strings.HasPrefix(value, "en-"):
		return English
	default:
		return ""
	}
}
