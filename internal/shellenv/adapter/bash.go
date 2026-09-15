package adapter

import (
	"os"
	"path/filepath"
	"runtime"
)

type Bash struct{}

func (Bash) Name() string   { return "bash" }
func (Bash) Syntax() string { return "posix" }

func (Bash) DefaultProfile(home, osName string) (string, string) {
	if osName == "windows" {
		return filepath.Join(home, ".bashrc"), "Windows bash default"
	}
	if osName == "darwin" {
		profile := filepath.Join(home, ".bash_profile")
		if _, err := os.Stat(profile); err == nil {
			return profile, "existing ~/.bash_profile"
		}
		return profile, "macOS login shell"
	}
	rc := filepath.Join(home, ".bashrc")
	if _, err := os.Stat(rc); err == nil {
		return rc, "existing ~/.bashrc"
	}
	if runtime.GOOS == "linux" {
		return rc, "Linux interactive shell"
	}
	return filepath.Join(home, ".bash_profile"), "fallback profile"
}

func (Bash) SourceCommand(profile string) string {
	return "source " + posixQuote(profile)
}

func (Bash) Render(variable, value string) (string, error) {
	if err := validateVariable(variable); err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", err
	}
	return "export " + variable + "=" + posixQuote(value), nil
}

func (Bash) Variable(line string) string {
	return posixVariable(line)
}
