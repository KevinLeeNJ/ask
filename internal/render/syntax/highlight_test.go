package syntax

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	code := "func main() {}\n"
	if got := Highlight(code, "go", false); got != code {
		t.Fatalf("color=false changed code: %q", got)
	}
	if got := Highlight(code, "go", true); !strings.Contains(got, "\x1b[") {
		t.Fatalf("color=true did not produce ANSI: %q", got)
	}
}

func TestHighlightCuratedLanguages(t *testing.T) {
	for _, language := range []string{
		"bash",
		"c",
		"cpp",
		"csharp",
		"css",
		"dockerfile",
		"go",
		"html",
		"java",
		"javascript",
		"json",
		"kotlin",
		"python",
		"rust",
		"sql",
		"terraform",
		"typescript",
		"yaml",
	} {
		t.Run(language, func(t *testing.T) {
			if got := Highlight("value := 1\n", language, true); got == "value := 1\n" {
				t.Fatalf("language %q was not highlighted", language)
			}
		})
	}
}

func TestHighlightAnalysesUnknownLanguage(t *testing.T) {
	code := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n"
	if got := Highlight(code, "", true); !strings.Contains(got, "\x1b[") {
		t.Fatalf("automatic language detection did not produce ANSI: %q", got)
	}
}
