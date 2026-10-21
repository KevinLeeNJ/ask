package syntax

import (
	"embed"
	"strings"

	"github.com/alecthomas/chroma/v2"
)

//go:embed embedded/*.xml
var embeddedFiles embed.FS

var lexerRegistry = newLexerRegistry()

// Keep the embedded set intentionally small. The assets are from Chroma
// v2.27.0 and retain their license in embedded/CHROMA_LICENSE.
var embeddedLexers = []string{
	"bash.xml",
	"c.xml",
	"c++.xml",
	"c#.xml",
	"css.xml",
	"diff.xml",
	"docker.xml",
	"html.xml",
	"ini.xml",
	"java.xml",
	"javascript.xml",
	"json.xml",
	"kotlin.xml",
	"lua.xml",
	"makefile.xml",
	"perl.xml",
	"php.xml",
	"powershell.xml",
	"python.xml",
	"r.xml",
	"ruby.xml",
	"rust.xml",
	"scala.xml",
	"sql.xml",
	"swift.xml",
	"terraform.xml",
	"toml.xml",
	"typescript.xml",
	"xml.xml",
	"yaml.xml",
}

func newLexerRegistry() *chroma.LexerRegistry {
	registry := chroma.NewLexerRegistry()
	for _, name := range embeddedLexers {
		registry.Register(chroma.MustNewXMLLexer(embeddedFiles, "embedded/"+name))
	}
	registry.Register(goLexer())
	return registry
}

// Chroma's Go lexer is defined in code rather than XML. Keep the same common
// token rules while avoiding Chroma's embedded global registry.
func goLexer() chroma.Lexer {
	return chroma.MustNewLexer(
		&chroma.Config{
			Name:      "Go",
			Aliases:   []string{"go", "golang"},
			Filenames: []string{"*.go"},
			MimeTypes: []string{"text/x-gosrc"},
		},
		func() chroma.Rules {
			return chroma.Rules{
				"root": {
					{Pattern: `\n`, Type: chroma.TextWhitespace},
					{Pattern: `\s+`, Type: chroma.TextWhitespace},
					{Pattern: `//[^\s\n\r][^\n\r]*`, Type: chroma.CommentPreproc},
					{Pattern: `//[^\n\r]*`, Type: chroma.CommentSingle},
					{Pattern: `/(\\\n)?[*](.|\n)*?[*](\\\n)?/`, Type: chroma.CommentMultiline},
					{Pattern: `(import|package)\b`, Type: chroma.KeywordNamespace},
					{Pattern: `(var|func|struct|map|chan|type|interface|const)\b`, Type: chroma.KeywordDeclaration},
					{
						Pattern: chroma.Words(``, `\b`, `break`, `default`, `select`, `case`, `defer`, `go`, `else`, `goto`, `switch`, `fallthrough`, `if`, `range`, `continue`, `for`, `return`),
						Type:    chroma.Keyword,
					},
					{Pattern: `(true|false|iota|nil)\b`, Type: chroma.KeywordConstant},
					{
						Pattern: chroma.Words(``, `\b(\()`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `int`, `int8`, `int16`, `int32`, `int64`, `float`, `float32`, `float64`, `complex64`, `complex128`, `byte`, `rune`, `string`, `bool`, `error`, `uintptr`, `print`, `println`, `panic`, `recover`, `close`, `complex`, `real`, `imag`, `len`, `cap`, `append`, `copy`, `delete`, `new`, `make`, `clear`, `min`, `max`),
						Type:    chroma.ByGroups(chroma.NameBuiltin, chroma.Punctuation),
					},
					{
						Pattern: chroma.Words(``, `\b`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `int`, `int8`, `int16`, `int32`, `int64`, `float`, `float32`, `float64`, `complex64`, `complex128`, `byte`, `rune`, `string`, `bool`, `error`, `uintptr`, `any`),
						Type:    chroma.KeywordType,
					},
					{Pattern: `\d+i`, Type: chroma.LiteralNumber},
					{Pattern: `\d+\.\d*([Ee][-+]\d+)?i`, Type: chroma.LiteralNumber},
					{Pattern: `\.\d+([Ee][-+]\d+)?i`, Type: chroma.LiteralNumber},
					{Pattern: `\d+[Ee][-+]\d+i`, Type: chroma.LiteralNumber},
					{Pattern: `\d+(\.\d+[eE][+\-]?\d+|\.\d*|[eE][+\-]?\d+)`, Type: chroma.LiteralNumberFloat},
					{Pattern: `\.\d+([eE][+\-]?\d+)?`, Type: chroma.LiteralNumberFloat},
					{Pattern: `0[0-7]+`, Type: chroma.LiteralNumberOct},
					{Pattern: `0[xX][0-9a-fA-F_]+`, Type: chroma.LiteralNumberHex},
					{Pattern: `0b[01_]+`, Type: chroma.LiteralNumberBin},
					{Pattern: `(0|[1-9][0-9_]*)`, Type: chroma.LiteralNumberInteger},
					{Pattern: `'(\\['"\\abfnrtv]|\\x[0-9a-fA-F]{2}|\\[0-7]{1,3}|\\u[0-9a-fA-F]{4}|\\U[0-9a-fA-F]{8}|[^\\])'`, Type: chroma.LiteralStringChar},
					{Pattern: "(`)([^`]*)(`)", Type: chroma.ByGroups(chroma.LiteralString, chroma.LiteralString, chroma.LiteralString)},
					{Pattern: `"(\\\\|\\"|[^"])*"`, Type: chroma.LiteralString},
					{Pattern: `(<<=|>>=|<<|>>|<=|>=|&\^=|&\^|\+=|-=|\*=|/=|%=|&=|\|=|&&|\|\||<-|\+\+|--|==|!=|:=|\.\.\.|[+\-*/%&])`, Type: chroma.Operator},
					{Pattern: `([a-zA-Z_]\w*)(\s*)(\()`, Type: chroma.ByGroups(chroma.NameFunction, chroma.UsingSelf("root"), chroma.Punctuation)},
					{Pattern: `[|^<>=!()\[\]{}.,;:~]`, Type: chroma.Punctuation},
					{Pattern: `[^\W\d]\w*`, Type: chroma.NameOther},
				},
			}
		},
	).SetAnalyser(func(text string) float32 {
		if strings.Contains(text, "fmt.") && strings.Contains(text, "package ") {
			return 0.5
		}
		if strings.Contains(text, "package ") {
			return 0.1
		}
		return 0
	})
}
