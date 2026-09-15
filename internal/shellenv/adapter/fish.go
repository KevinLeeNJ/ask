package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

type Fish struct{}

func (Fish) Name() string   { return "fish" }
func (Fish) Syntax() string { return "fish" }

func (Fish) DefaultProfile(home, osName string) (string, string) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "fish", "config.fish"), "fish default"
}

func (Fish) SourceCommand(profile string) string {
	return "source " + posixQuote(profile)
}

func (Fish) Render(variable, value string) (string, error) {
	if err := validateVariable(variable); err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", err
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "set -gx " + variable + " '" + escaped + "'", nil
}

func (Fish) Variable(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "set -gx ")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
