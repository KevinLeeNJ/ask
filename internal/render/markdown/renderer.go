package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/KevinLeeNJ/ask/internal/render/syntax"
)

const (
	reset      = "\x1b[0m"
	bold       = "\x1b[1m"
	italic     = "\x1b[3m"
	strike     = "\x1b[9m"
	cyan       = "\x1b[36m"
	inlineCode = "\x1b[48;5;236;38;5;252m"
	quoteColor = "\x1b[90m"
)

type Options struct {
	Color      bool
	Hyperlinks bool
	Width      int
}

func Render(source []byte, options Options) string {
	if strings.TrimSpace(string(source)) == "" {
		return ""
	}
	parser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	).Parser()
	document := parser.Parse(text.NewReader(source))
	renderer := &renderer{
		source:  source,
		options: options,
		lists:   map[*ast.List]int{},
	}
	if err := ast.Walk(document, renderer.walk); err != nil {
		return string(source)
	}
	return normalize(renderer.out.String())
}

type renderer struct {
	source         []byte
	options        Options
	out            strings.Builder
	quoteDepth     int
	listDepth      int
	lists          map[*ast.List]int
	lineStart      bool
	atParagraphEnd bool
}

func (r *renderer) walk(node ast.Node, entering bool) (ast.WalkStatus, error) {
	switch typed := node.(type) {
	case *ast.Document:
	case *ast.Heading:
		if entering {
			r.blankLine()
			if r.options.Color {
				switch typed.Level {
				case 1:
					r.writeRaw(bold + cyan)
				case 2:
					r.writeRaw(bold)
				default:
					r.writeRaw(cyan)
				}
			}
		} else {
			if r.options.Color {
				r.writeRaw(reset)
			}
			r.ensureNewline()
		}
	case *ast.Paragraph:
		if entering {
			if r.listDepth == 0 {
				r.blankLine()
			}
		} else {
			r.ensureNewline()
		}
	case *ast.Text:
		if entering {
			r.writeInline(string(typed.Segment.Value(r.source)))
			if typed.SoftLineBreak() || typed.HardLineBreak() {
				r.ensureNewline()
			}
		}
	case *ast.String:
		if entering {
			r.writeInline(string(typed.Value))
		}
	case *ast.CodeSpan:
		if entering {
			text := string(typed.Text(r.source))
			if r.options.Color {
				r.writeRaw(inlineCode + text + reset)
			} else {
				r.writeInline(text)
			}
			return ast.WalkSkipChildren, nil
		}
	case *ast.Emphasis:
		if r.options.Color {
			if entering {
				if typed.Level >= 2 {
					r.writeRaw(bold)
				} else {
					r.writeRaw(italic)
				}
			} else {
				r.writeRaw(reset)
			}
		}
	case *ast.FencedCodeBlock:
		if !entering {
			return ast.WalkContinue, nil
		}
		r.blankLine()
		code := linesText(typed.Lines(), r.source)
		language := string(typed.Language(r.source))
		r.writeRaw(strings.TrimSuffix(syntax.Highlight(code, language, r.options.Color), "\n"))
		r.blankLine()
		return ast.WalkSkipChildren, nil
	case *ast.CodeBlock:
		if !entering {
			return ast.WalkContinue, nil
		}
		r.blankLine()
		code := linesText(typed.Lines(), r.source)
		r.writeRaw(strings.TrimSuffix(syntax.Highlight(code, "", r.options.Color), "\n"))
		r.blankLine()
		return ast.WalkSkipChildren, nil
	case *ast.Blockquote:
		if entering {
			r.blankLine()
			r.quoteDepth++
		} else {
			r.quoteDepth--
			r.blankLine()
		}
	case *ast.List:
		if entering {
			r.blankLine()
			r.lists[typed] = typed.Start - 1
			r.listDepth++
		} else {
			r.listDepth--
			r.blankLine()
		}
	case *ast.ListItem:
		if entering {
			r.writeRaw(strings.Repeat("  ", max(0, r.listDepth-1)))
			parent, _ := typed.Parent().(*ast.List)
			if parent != nil && parent.IsOrdered() {
				r.lists[parent]++
				r.writeRaw(fmt.Sprintf("%d. ", r.lists[parent]))
			} else {
				r.writeRaw("• ")
			}
		} else {
			r.ensureNewline()
		}
	case *ast.ThematicBreak:
		if entering {
			r.blankLine()
			width := r.options.Width
			if width <= 0 {
				width = 80
			}
			r.writeRaw(strings.Repeat("─", width))
			r.blankLine()
		}
	case *ast.Link:
		if entering {
			if r.options.Hyperlinks && r.options.Color {
				r.writeRaw("\x1b]8;;" + string(typed.Destination) + "\x1b\\")
			}
		} else {
			if r.options.Hyperlinks && r.options.Color {
				r.writeRaw("\x1b]8;;\x1b\\")
			} else {
				r.writeInline(" (" + string(typed.Destination) + ")")
			}
		}
	case *ast.AutoLink:
		if entering {
			label := string(typed.Label(r.source))
			url := string(typed.URL(r.source))
			if r.options.Hyperlinks && r.options.Color {
				r.writeRaw("\x1b]8;;" + url + "\x1b\\" + label + "\x1b]8;;\x1b\\")
				return ast.WalkSkipChildren, nil
			}
			r.writeInline(label)
			if label != url {
				r.writeInline(" (" + url + ")")
			}
			return ast.WalkSkipChildren, nil
		}
	case *ast.Image:
		if entering {
			r.writeInline("image: ")
		} else {
			r.writeInline(" (" + string(typed.Destination) + ")")
		}
	case *ast.RawHTML:
		if entering {
			for index := 0; index < typed.Segments.Len(); index++ {
				segment := typed.Segments.At(index)
				r.writeInline(string(segment.Value(r.source)))
			}
			return ast.WalkSkipChildren, nil
		}
	case *ast.HTMLBlock:
		if entering {
			r.writeRaw(linesText(typed.Lines(), r.source))
			r.ensureNewline()
			return ast.WalkSkipChildren, nil
		}
	case *east.Table:
		if entering {
			r.blankLine()
		} else {
			r.blankLine()
		}
	case *east.TableHeader:
		if !entering {
			r.ensureNewline()
			cells := countDirectChildren(typed, east.KindTableCell)
			if cells == 0 {
				cells = 1
			}
			r.writeRaw("|" + strings.Repeat(" ─── |", cells))
			r.ensureNewline()
		}
	case *east.TableRow:
		if !entering {
			r.ensureNewline()
		}
	case *east.TableCell:
		if entering {
			r.writeRaw("| ")
		} else {
			r.writeRaw(" ")
		}
	case *ast.TextBlock:
		if !entering {
			r.ensureNewline()
		}
	default:
	}
	return ast.WalkContinue, nil
}

func (r *renderer) writeInline(text string) {
	r.writeRaw(text)
}

func (r *renderer) writeRaw(text string) {
	if text == "" {
		return
	}
	if r.quoteDepth > 0 && r.lineStart {
		r.out.WriteString(strings.Repeat("│ ", r.quoteDepth))
		r.lineStart = false
	}
	r.out.WriteString(text)
	if strings.HasSuffix(text, "\n") {
		r.lineStart = true
	}
}

func (r *renderer) ensureNewline() {
	value := r.out.String()
	if value == "" || strings.HasSuffix(value, "\n") {
		r.lineStart = true
		return
	}
	r.out.WriteByte('\n')
	r.lineStart = true
}

func (r *renderer) blankLine() {
	r.ensureNewline()
	if r.out.Len() > 0 && !strings.HasSuffix(r.out.String(), "\n\n") {
		r.out.WriteByte('\n')
	}
	r.lineStart = true
}

func normalize(input string) string {
	lines := strings.Split(input, "\n")
	output := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmedRight) == "" {
			if blank {
				continue
			}
			blank = true
			output = append(output, "")
			continue
		}
		blank = false
		output = append(output, trimmedRight)
	}
	for len(output) > 0 && output[len(output)-1] == "" {
		output = output[:len(output)-1]
	}
	return strings.Join(output, "\n") + "\n"
}

func linesText(lines *text.Segments, source []byte) string {
	var builder strings.Builder
	for index := 0; index < lines.Len(); index++ {
		segment := lines.At(index)
		builder.Write(segment.Value(source))
	}
	if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteByte('\n')
	}
	return builder.String()
}

func countDirectChildren(node ast.Node, kind ast.NodeKind) int {
	count := 0
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == kind {
			count++
		}
	}
	return count
}
