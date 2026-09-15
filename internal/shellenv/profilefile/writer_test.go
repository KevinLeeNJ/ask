package profilefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
)

func TestWriterPreviewWriteBackupAndConflict(t *testing.T) {
	profile := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(profile, []byte("export EXISTING='yes'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer := Writer{}
	change := ports.ShellProfileChange{
		Shell:        "zsh",
		ProfileFile:  profile,
		Variable:     "OPENAI_API_KEY",
		Value:        "secret'value",
		ManagedBlock: true,
	}
	preview, err := writer.Preview(change)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.Conflict || preview.Syntax != "posix" || !strings.Contains(preview.Content, "ask managed secrets") {
		t.Fatalf("preview = %#v", preview)
	}
	if strings.Contains(preview.Content, "\nsecret'value\n") {
		t.Fatalf("secret was not escaped:\n%s", preview.Content)
	}
	written, err := writer.Write(change)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	content, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "OPENAI_API_KEY") || !strings.Contains(string(content), "EXISTING") {
		t.Fatalf("profile content:\n%s", content)
	}
	info, err := os.Stat(profile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("profile permissions = %o", info.Mode().Perm())
	}
	if _, err := os.Stat(written.BackupFile); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	removed, err := writer.Remove(change)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if strings.Contains(removed.Content, "OPENAI_API_KEY") || !strings.Contains(removed.Content, "EXISTING") {
		t.Fatalf("removed content:\n%s", removed.Content)
	}

	conflictProfile := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(conflictProfile, []byte("export OPENAI_API_KEY='outside'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	change.ProfileFile = conflictProfile
	preview, err = writer.Preview(change)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Conflict {
		t.Fatal("expected conflict")
	}
	if _, err := writer.Write(change); err == nil {
		t.Fatal("conflicting write unexpectedly succeeded")
	}
	change.Overwrite = true
	if _, err := writer.Write(change); err != nil {
		t.Fatalf("overwrite Write() error = %v", err)
	}
}
