package syntax

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

func Highlight(code, language string, color bool) string {
	if !color || strings.TrimSpace(code) == "" {
		return code
	}
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		return code
	}
	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		return code
	}
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var output bytes.Buffer
	if err := formatter.Format(&output, style, iterator); err != nil {
		return code
	}
	return output.String()
}
