package syntax

import (
	"bytes"
	"strings"
)

func Highlight(code, language string, color bool) string {
	if !color || strings.TrimSpace(code) == "" {
		return code
	}
	lexer := lexerRegistry.Get(language)
	if lexer == nil {
		lexer = lexerRegistry.Analyse(code)
	}
	if lexer == nil {
		return code
	}
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var output bytes.Buffer
	if err := terminal16m.Format(&output, githubDarkStyle, iterator); err != nil {
		return code
	}
	return output.String()
}
