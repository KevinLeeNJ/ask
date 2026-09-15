package adapter

import (
	"strings"
	"testing"
)

func TestRenderEscapesShellValues(t *testing.T) {
	tests := []struct {
		name     string
		adapter  Adapter
		value    string
		contains string
	}{
		{name: "zsh", adapter: Zsh{}, value: "a'b\\c", contains: `export KEY='a'\''b\c'`},
		{name: "fish", adapter: Fish{}, value: "a'b\\c", contains: `set -gx KEY 'a\'b\\c'`},
		{name: "powershell", adapter: PowerShell{}, value: "a'b", contains: `$env:KEY = 'a''b'`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.adapter.Render("KEY", test.value)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if got != test.contains {
				t.Fatalf("Render() = %q, want %q", got, test.contains)
			}
		})
	}
}

func TestRenderRejectsControlCharacters(t *testing.T) {
	if _, err := (Zsh{}).Render("KEY", "value\nnext"); err == nil {
		t.Fatal("expected control-character error")
	}
	if _, err := (Zsh{}).Render("bad-name", "value"); err == nil {
		t.Fatal("expected variable-name error")
	}
}

func TestDetect(t *testing.T) {
	for _, shell := range []string{"/bin/zsh", "/bin/bash", "/usr/bin/fish", "/bin/sh", "pwsh"} {
		if _, err := Detect(shell, "/home/user", "linux"); err != nil {
			t.Errorf("Detect(%q) error = %v", shell, err)
		}
	}
	if _, err := Detect("/bin/unknown", "/home/user", "linux"); err == nil || !strings.Contains(err.Error(), "unknown shell") {
		t.Fatalf("Detect(unknown) error = %v", err)
	}
}
