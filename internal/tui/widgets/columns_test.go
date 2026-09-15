package widgets

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFormatColumnsAlignsWideText(t *testing.T) {
	widths := []int{12, 10, 8}
	header := FormatColumns([]string{"标题", "更新时间", "消息数"}, widths)
	row := FormatColumns([]string{"短标题", "10 小时前", "21"}, widths)

	if got, want := columnOffset(row, "10 小时前"), columnOffset(header, "更新时间"); got != want {
		t.Fatalf("updated column offset = %d, want %d", got, want)
	}
	if got, want := columnOffset(row, "21"), columnOffset(header, "消息数"); got != want {
		t.Fatalf("messages column offset = %d, want %d", got, want)
	}
}

func TestFormatColumnsTruncatesWithoutBreakingWidth(t *testing.T) {
	got := FormatColumns([]string{"abcdefgh", "x"}, []int{5, 1})
	if got != "abcd…  x" {
		t.Fatalf("FormatColumns() = %q, want %q", got, "abcd…  x")
	}
}

func TestFormatCompactFitsRequestedWidth(t *testing.T) {
	got := FormatCompact([]string{"provider-name", "OpenAI Responses", "model-name"}, 50)
	if width := ansi.StringWidth(got); width > 50 {
		t.Fatalf("FormatCompact() width = %d, want <= 50: %q", width, got)
	}
}

func TestCompactHeaderFitsRequestedWidth(t *testing.T) {
	got := CompactHeader([]string{"Title", "Updated", "Messages"}, 20)
	if width := ansi.StringWidth(got); width > 20 {
		t.Fatalf("CompactHeader() width = %d, want <= 20: %q", width, got)
	}
}

func columnOffset(line, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	return ansi.StringWidth(line[:index])
}
