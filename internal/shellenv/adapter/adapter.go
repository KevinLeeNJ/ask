package adapter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Adapter interface {
	Name() string
	Syntax() string
	DefaultProfile(home, osName string) (path string, reason string)
	SourceCommand(profile string) string
	Render(variable, value string) (string, error)
	Variable(line string) string
}

func Detect(shellPath, home, osName string) (Adapter, error) {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(shellPath)))
	switch {
	case name == "zsh":
		return Zsh{}, nil
	case name == "bash":
		return Bash{}, nil
	case name == "fish":
		return Fish{}, nil
	case name == "sh" || name == "dash" || name == "ash":
		return POSIX{}, nil
	case strings.Contains(name, "powershell") || name == "pwsh":
		return PowerShell{}, nil
	default:
		return nil, fmt.Errorf("unknown shell %q", shellPath)
	}
}

func ByName(name string) (Adapter, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "zsh":
		return Zsh{}, nil
	case "bash":
		return Bash{}, nil
	case "fish":
		return Fish{}, nil
	case "sh", "posix", "dash", "ash":
		return POSIX{}, nil
	case "powershell", "pwsh":
		return PowerShell{}, nil
	default:
		return nil, fmt.Errorf("unknown shell %q", name)
	}
}

func validateVariable(variable string) error {
	if !variablePattern.MatchString(variable) {
		return fmt.Errorf("invalid environment variable name")
	}
	return nil
}

func validateValue(value string) error {
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("API key contains an invalid control character")
	}
	return nil
}

func posixQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
