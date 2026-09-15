package configdir

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestAskDirUsesHomeConfigOnMacAndLinux(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("home config directory rule applies to macOS and Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := AskDir()
	if err != nil {
		t.Fatalf("AskDir() error = %v", err)
	}
	want := filepath.Join(home, ".config", "ask")
	if got != want {
		t.Fatalf("AskDir() = %q, want %q", got, want)
	}
}
