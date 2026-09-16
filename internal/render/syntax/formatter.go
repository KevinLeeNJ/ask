package syntax

import (
	"fmt"
	"io"
	"regexp"

	"github.com/alecthomas/chroma/v2"
)

var (
	githubDarkStyle = loadStyle("embedded/github-dark.xml")
	terminal16m     = chroma.FormatterFunc(formatTerminal16m)
	crOrCrLf        = regexp.MustCompile(`\r?\n`)
)

func loadStyle(path string) *chroma.Style {
	file, err := embeddedFiles.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	style, err := chroma.NewXMLStyle(file)
	if err != nil {
		panic(err)
	}
	return style
}

// formatTerminal16m mirrors Chroma's terminal16m formatter without pulling in
// its HTML, SVG, or indexed terminal formatters.
func formatTerminal16m(writer io.Writer, style *chroma.Style, iterator chroma.Iterator) error {
	style = withoutBackground(style)
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		if entry.IsZero() {
			fmt.Fprint(writer, token.Value)
			continue
		}

		formatting := ""
		if entry.Bold == chroma.Yes {
			formatting += "\033[1m"
		}
		if entry.Underline == chroma.Yes {
			formatting += "\033[4m"
		}
		if entry.Italic == chroma.Yes {
			formatting += "\033[3m"
		}
		if entry.Colour.IsSet() {
			formatting += fmt.Sprintf(
				"\033[38;2;%d;%d;%dm",
				entry.Colour.Red(),
				entry.Colour.Green(),
				entry.Colour.Blue(),
			)
		}
		if entry.Background.IsSet() {
			formatting += fmt.Sprintf(
				"\033[48;2;%d;%d;%dm",
				entry.Background.Red(),
				entry.Background.Green(),
				entry.Background.Blue(),
			)
		}
		writeToken(writer, formatting, token.Value)
	}
	return nil
}

func writeToken(writer io.Writer, formatting, text string) {
	if formatting == "" {
		fmt.Fprint(writer, text)
		return
	}
	newlineIndices := crOrCrLf.FindAllStringIndex(text, -1)
	afterLastNewline := 0
	for _, indices := range newlineIndices {
		newlineStart, afterNewline := indices[0], indices[1]
		fmt.Fprint(writer, formatting)
		fmt.Fprint(writer, text[afterLastNewline:newlineStart])
		fmt.Fprint(writer, "\033[0m")
		fmt.Fprint(writer, text[newlineStart:afterNewline])
		afterLastNewline = afterNewline
	}
	if afterLastNewline < len(text) {
		fmt.Fprint(writer, formatting)
		fmt.Fprint(writer, text[afterLastNewline:])
		fmt.Fprint(writer, "\033[0m")
	}
}

func withoutBackground(style *chroma.Style) *chroma.Style {
	builder := style.Builder()
	background := builder.Get(chroma.Background)
	background.Background = 0
	background.NoInherit = true
	builder.AddEntry(chroma.Background, background)
	style, _ = builder.Build()
	return style
}
