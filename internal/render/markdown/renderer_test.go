package markdown

import (
	"strings"
	"testing"
)

func TestRenderSemanticMarkdownWithoutRawMarkers(t *testing.T) {
	source := []byte("# Heading\n\n**bold** and *italic*\n\n- one\n- two\n\n> quote\n\n`inline`\n\n[link](https://example.com)\n")
	got := Render(source, Options{Color: false, Hyperlinks: false, Width: 20})
	for _, forbidden := range []string{"# Heading", "**bold**", "*italic*", "> quote", "`inline`"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("rendered output contains raw marker %q:\n%s", forbidden, got)
		}
	}
	for _, want := range []string{"Heading", "bold", "italic", "• one", "│ quote", "inline", "https://example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPreservesCodeBlockLiterals(t *testing.T) {
	source := []byte("```go\n# literal\n**literal**\n`literal`\n```\n")
	got := Render(source, Options{Color: false, Width: 40})
	for _, literal := range []string{"# literal", "**literal**", "`literal`"} {
		if !strings.Contains(got, literal) {
			t.Fatalf("code literal %q was changed:\n%s", literal, got)
		}
	}
}

func TestRenderTableAndImageFallback(t *testing.T) {
	source := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n\n![alt](https://example.com/a.png)\n")
	got := Render(source, Options{Color: false, Width: 40})
	if strings.Contains(got, "| --- |") {
		t.Fatalf("raw table separator leaked:\n%s", got)
	}
	if !strings.Contains(got, "| A ") || !strings.Contains(got, "image: alt (https://example.com/a.png)") {
		t.Fatalf("table/image fallback missing:\n%s", got)
	}
}
