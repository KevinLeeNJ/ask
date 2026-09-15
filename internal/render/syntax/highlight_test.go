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
