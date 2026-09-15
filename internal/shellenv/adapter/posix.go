package adapter

import (
	"path/filepath"
	"strings"
)

type POSIX struct{}

func (POSIX) Name() string   { return "sh" }
func (POSIX) Syntax() string { return "posix" }

func (POSIX) DefaultProfile(home, osName string) (string, string) {
	return filepath.Join(home, ".profile"), "POSIX sh default"
}

func (POSIX) SourceCommand(profile string) string {
	return ". " + posixQuote(profile)
}

func (POSIX) Render(variable, value string) (string, error) {
	if err := validateVariable(variable); err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", err
	}
	return "export " + variable + "=" + posixQuote(value), nil
}

func (POSIX) Variable(line string) string {
	return posixVariable(line)
}

func posixVariable(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "export ")
	name, _, _ := strings.Cut(line, "=")
	return strings.TrimSpace(name)
}
