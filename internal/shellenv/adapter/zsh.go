package adapter

import (
	"os"
	"path/filepath"
)

type Zsh struct{}

func (Zsh) Name() string   { return "zsh" }
func (Zsh) Syntax() string { return "posix" }

func (Zsh) DefaultProfile(home, osName string) (string, string) {
	if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
		return filepath.Join(zdotdir, ".zshrc"), "ZDOTDIR is set"
	}
	return filepath.Join(home, ".zshrc"), "zsh default"
}

func (Zsh) SourceCommand(profile string) string {
	return "source " + posixQuote(profile)
}

func (Zsh) Render(variable, value string) (string, error) {
	if err := validateVariable(variable); err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", err
	}
	return "export " + variable + "=" + posixQuote(value), nil
}

func (Zsh) Variable(line string) string {
	return posixVariable(line)
}
