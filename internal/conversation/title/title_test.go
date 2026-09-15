package title

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProvisionalNormalization(t *testing.T) {
	input := "  第一行\n\n第二行  空格  "
	if got := Provisional(input); got != "第一行 第二行 空格" {
		t.Fatalf("Provisional() = %q", got)
	}
	long := strings.Repeat("字", 40)
	got := Provisional(long)
	if !strings.HasSuffix(got, "...") || utf8.RuneCountInString(got) != 33 {
		t.Fatalf("truncated title = %q", got)
	}
}

func TestNormalizeGenerated(t *testing.T) {
	tests := map[string]string{
		"Title: **Go concurrency**": "Go concurrency",
		"标题：`SQLite 索引`。":           "SQLite 索引",
		"first line\nsecond line":   "first line",
		"  ":                        "",
	}
	for input, want := range tests {
		if got := NormalizeGenerated(input); got != want {
			t.Errorf("NormalizeGenerated(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRequestTextLimit(t *testing.T) {
	got := RequestText(strings.Repeat("a", 2100))
	if utf8.RuneCountInString(got) != 2003 {
		t.Fatalf("request length = %d", utf8.RuneCountInString(got))
	}
}
