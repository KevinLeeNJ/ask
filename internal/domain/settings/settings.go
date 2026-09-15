package settings

import "time"

const (
	RetentionNever  = "never"
	Retention7Days  = "7d"
	Retention30Days = "30d"
	Retention60Days = "60d"
)

type Config struct {
	Version        int
	ActiveProvider string
	Model          Model
	Providers      map[string]Provider
	ShellEnv       ShellEnv
	History        History
	Chat           Chat
	Reasoning      Reasoning
	UI             UI
}

type Model struct {
	AutoDiscover    bool
	Temperature     float64
	MaxOutputTokens int
}

type Provider struct {
	Format         string
	BaseURL        string
	DefaultModel   string
	EnabledModels  []string
	RequestTimeout time.Duration
	Headers        map[string]string
}

type ShellEnv struct {
	Shell        string
	Syntax       string
	ProfileFile  string
	ManagedBlock bool
}

type History struct {
	Retention string
}

type Chat struct {
	SystemPrompt       string
	MaxContextMessages int
	MaxContextTokens   int
	Stream             bool
}

type Reasoning struct {
	Mode                     string
	Strategy                 string
	RequestField             string
	EnabledValue             string
	DisabledValue            string
	DisabledBehavior         string
	Show                     string
	CollapseWhenAnswerStarts bool
	AnthropicBudgetTokens    string
}

type UI struct {
	Language        string
	RenderMode      string
	Color           string
	Markdown        string
	MarkdownTheme   string
	SyntaxHighlight bool
	Hyperlinks      string
	ShowUsage       bool
}

func Default() Config {
	return Config{
		Version: 1,
		Model: Model{
			AutoDiscover:    true,
			Temperature:     0.7,
			MaxOutputTokens: 4096,
		},
		Providers: map[string]Provider{},
		ShellEnv: ShellEnv{
			ManagedBlock: true,
		},
		History: History{
			Retention: RetentionNever,
		},
		Chat: Chat{
			MaxContextMessages: 40,
			MaxContextTokens:   32000,
			Stream:             true,
		},
		Reasoning: Reasoning{
			Mode:                     string(ReasoningAuto),
			Strategy:                 "auto",
			RequestField:             "reasoning_effort",
			EnabledValue:             "minimum",
			DisabledValue:            "none",
			DisabledBehavior:         "send",
			Show:                     "auto",
			CollapseWhenAnswerStarts: true,
			AnthropicBudgetTokens:    "minimum",
		},
		UI: UI{
			Language:        "auto",
			RenderMode:      "auto",
			Color:           "auto",
			Markdown:        "auto",
			MarkdownTheme:   "auto",
			SyntaxHighlight: true,
			Hyperlinks:      "auto",
			ShowUsage:       true,
		},
	}
}

type ReasoningMode string

const (
	ReasoningAuto ReasoningMode = "auto"
	ReasoningOn   ReasoningMode = "on"
	ReasoningOff  ReasoningMode = "off"
)
