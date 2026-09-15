package widgets

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const columnGap = "  "

// FormatColumns renders cells using fixed terminal-column widths.
func FormatColumns(cells []string, widths []int) string {
	if len(cells) != len(widths) {
		panic("widgets: cells and widths must have the same length")
	}

	var line strings.Builder
	for index, cell := range cells {
		if index > 0 {
			line.WriteString(columnGap)
		}

		width := max(widths[index], 0)
		cell = ansi.Truncate(cell, width, "…")
		line.WriteString(cell)
		if index == len(cells)-1 {
			continue
		}
		if padding := width - ansi.StringWidth(cell); padding > 0 {
			line.WriteString(strings.Repeat(" ", padding))
		}
	}
	return line.String()
}

// FormatCompact distributes the available terminal width across cells.
func FormatCompact(cells []string, width int) string {
	if len(cells) == 0 {
		return ""
	}
	width = max(width, len(cells))
	available := width - len(columnGap)*(len(cells)-1)
	if available < len(cells) {
		available = len(cells)
	}
	widths := make([]int, len(cells))
	base := available / len(cells)
	remainder := available % len(cells)
	for index := range widths {
		widths[index] = base
		if index < remainder {
			widths[index]++
		}
	}
	return FormatColumns(cells, widths)
}

// CompactHeader builds a single-line header that fits within width.
func CompactHeader(labels []string, width int) string {
	header := strings.Join(labels, " · ")
	return ansi.Truncate(header, max(width, 1), "…")
}
