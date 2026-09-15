package architecture_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func TestImportDirection(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate architecture test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list error = %v", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(output)))
	packages := make([]listedPackage, 0)
	for decoder.More() {
		var value listedPackage
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, value)
	}

	for _, pkg := range packages {
		relative := strings.TrimPrefix(pkg.ImportPath, "github.com/KevinLeeNJ/ask/")
		for _, imported := range pkg.Imports {
			if !strings.HasPrefix(imported, "github.com/KevinLeeNJ/ask/internal/") {
				continue
			}
			importedRelative := strings.TrimPrefix(imported, "github.com/KevinLeeNJ/ask/internal/")
			if strings.HasPrefix(relative, "internal/domain/") &&
				!strings.HasPrefix(importedRelative, "domain/") {
				t.Errorf("%s imports non-domain package %s", relative, imported)
			}

			if strings.HasPrefix(relative, "internal/application/") {
				if forbiddenApplicationImport(importedRelative) {
					t.Errorf("%s imports concrete outer-layer package %s", relative, imported)
				}
			}

			if strings.HasPrefix(relative, "internal/cli/") || strings.HasPrefix(relative, "internal/tui/") {
				if strings.HasPrefix(importedRelative, "storage/") ||
					strings.HasPrefix(importedRelative, "provider/openai/") ||
					strings.HasPrefix(importedRelative, "provider/anthropic/") ||
					strings.HasPrefix(importedRelative, "bootstrap/") {
					t.Errorf("%s imports concrete implementation %s", relative, imported)
				}
			}

			if strings.HasPrefix(relative, "internal/provider/") {
				if strings.HasPrefix(importedRelative, "storage/") ||
					strings.HasPrefix(importedRelative, "cli/") ||
					strings.HasPrefix(importedRelative, "tui/") ||
					strings.HasPrefix(importedRelative, "render/") ||
					strings.HasPrefix(importedRelative, "config/") {
					t.Errorf("%s imports unrelated package %s", relative, imported)
				}
			}

			if strings.HasPrefix(relative, "internal/storage/") {
				if strings.HasPrefix(importedRelative, "cli/") ||
					strings.HasPrefix(importedRelative, "tui/") ||
					strings.HasPrefix(importedRelative, "provider/") ||
					strings.HasPrefix(importedRelative, "render/") {
					t.Errorf("%s imports presentation/provider package %s", relative, imported)
				}
			}
		}
	}
}

func forbiddenApplicationImport(imported string) bool {
	prefixes := []string{
		"cli/",
		"tui/",
		"storage/",
		"provider/openai/",
		"provider/anthropic/",
		"provider/registry/",
		"render/",
		"shellenv/",
		"bootstrap/",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(imported, prefix) {
			return true
		}
	}
	return false
}
