package adapter

import (
	"path/filepath"
	"strings"
)

type PowerShell struct{}

func (PowerShell) Name() string   { return "powershell" }
func (PowerShell) Syntax() string { return "powershell" }

func (PowerShell) DefaultProfile(home, osName string) (string, string) {
	if osName == "windows" {
		return filepath.Join(home, "Documents", "PowerShell", "profile.ps1"), "PowerShell CurrentUserAllHosts"
	}
	return filepath.Join(home, ".config", "powershell", "profile.ps1"), "PowerShell default"
}

func (PowerShell) SourceCommand(profile string) string {
	return ". '" + strings.ReplaceAll(profile, "'", "''") + "'"
}

func (PowerShell) Render(variable, value string) (string, error) {
	if err := validateVariable(variable); err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", err
	}
	escaped := strings.ReplaceAll(value, "'", "''")
	return "$env:" + variable + " = '" + escaped + "'", nil
}

func (PowerShell) Variable(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "$env:")
	name, _, _ := strings.Cut(line, "=")
	return strings.TrimSpace(name)
}
