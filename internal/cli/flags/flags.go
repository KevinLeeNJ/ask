package flags

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

type Command string

const (
	CommandAsk           Command = "ask"
	CommandConfig        Command = "config"
	CommandConversations Command = "conversations"
	CommandHelp          Command = "help"
	CommandVersion       Command = "version"
)

type Options struct {
	Command             Command
	Question            string
	ConversationID      string
	ProviderOverride    string
	ModelOverride       string
	SystemOverride      *string
	LanguageOverride    string
	ThinkingOverride    *provider.ThinkingMode
	ConversationNew     bool
	ShowThinking        bool
	HideThinking        bool
	NoStream            bool
	Markdown            *bool
	NoColor             bool
	RenderMode          string
	JSON                bool
	Quiet               bool
	MaxContextMessages  int
	TimeoutOverride     *time.Duration
	ConversationsAction string
	ConversationsID     string
}

type parser struct {
	options         Options
	positionals     []string
	seen            map[string]bool
	afterSeparator  bool
	separatorFirst  string
	separatorRemain []string
}

func Parse(args []string) (Options, error) {
	p := &parser{
		options: Options{
			Command:          CommandAsk,
			LanguageOverride: "auto",
		},
		seen: map[string]bool{},
	}
	if err := p.parse(args); err != nil {
		return p.options, err
	}
	if p.options.Command == CommandAsk {
		p.options.Question = strings.Join(p.positionals, " ")
	}
	return p.options, nil
}

func (p *parser) parse(args []string) error {
	for index := 0; index < len(args); index++ {
		token := args[index]
		if p.afterSeparator {
			p.positionals = append(p.positionals, token)
			continue
		}
		if token == "--" {
			p.afterSeparator = true
			p.separatorRemain = append([]string(nil), args[index+1:]...)
			if len(p.separatorRemain) > 0 {
				p.separatorFirst = p.separatorRemain[0]
			}
			continue
		}

		name, inlineValue, hasInlineValue := splitFlag(token)
		switch name {
		case "-h", "--help":
			p.options.Command = CommandHelp
			p.seen["help"] = true
			continue
		case "--version":
			p.options.Command = CommandVersion
			p.seen["version"] = true
			continue
		case "-n", "--new":
			p.options.ConversationNew = true
			p.seen["new"] = true
			continue
		case "--thinking":
			value := provider.ThinkingOn
			p.options.ThinkingOverride = &value
			p.seen["thinking"] = true
			continue
		case "--no-thinking":
			value := provider.ThinkingOff
			p.options.ThinkingOverride = &value
			p.seen["no-thinking"] = true
			continue
		case "--show-thinking":
			p.options.ShowThinking = true
			p.seen["show-thinking"] = true
			continue
		case "--hide-thinking":
			p.options.HideThinking = true
			p.seen["hide-thinking"] = true
			continue
		case "--no-stream":
			p.options.NoStream = true
			p.seen["no-stream"] = true
			continue
		case "--markdown":
			value := true
			p.options.Markdown = &value
			p.seen["markdown"] = true
			continue
		case "--no-markdown":
			value := false
			p.options.Markdown = &value
			p.seen["no-markdown"] = true
			continue
		case "--no-color":
			p.options.NoColor = true
			p.seen["no-color"] = true
			continue
		case "--json":
			p.options.JSON = true
			p.seen["json"] = true
			continue
		case "--quiet":
			p.options.Quiet = true
			p.seen["quiet"] = true
			continue
		case "-c", "--conversation":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			p.options.ConversationID = value
			p.seen["conversation"] = true
			continue
		case "-p", "--provider":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			p.options.ProviderOverride = value
			p.seen["provider"] = true
			continue
		case "-m", "--model":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			p.options.ModelOverride = value
			p.seen["model"] = true
			continue
		case "--lang":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			switch value {
			case "auto", "zh-CN", "en-US":
				p.options.LanguageOverride = value
			default:
				return invalidValue(name, value, "auto、zh-CN or en-US")
			}
			p.seen["lang"] = true
			continue
		case "--system":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			p.options.SystemOverride = &value
			p.seen["system"] = true
			continue
		case "--render-mode":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			switch value {
			case "auto", "full", "inline", "plain":
				p.options.RenderMode = value
			default:
				return invalidValue(name, value, "auto, full, inline, or plain")
			}
			p.seen["render-mode"] = true
			continue
		case "--max-context-messages":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			number, err := strconv.Atoi(value)
			if err != nil || number <= 0 {
				return invalidValue(name, value, "a positive integer")
			}
			p.options.MaxContextMessages = number
			p.seen["max-context-messages"] = true
			continue
		case "--timeout":
			value, err := p.takeValue(args, &index, name, inlineValue, hasInlineValue)
			if err != nil {
				return err
			}
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return invalidValue(name, value, "a positive duration such as 30s")
			}
			p.options.TimeoutOverride = &duration
			p.seen["timeout"] = true
			continue
		}

		p.positionals = append(p.positionals, token)
	}

	if p.afterSeparator && p.separatorFirst != "" && strings.HasPrefix(p.separatorFirst, "-") {
		correction := buildCorrection(p.separatorFirst, p.separatorRemain[1:])
		return failure.New(
			failure.KindUsage,
			"usage.separator_option",
			map[string]string{
				"token":      p.separatorFirst,
				"correction": correction,
			},
		)
	}

	if err := p.validateConflicts(); err != nil {
		return err
	}
	if p.options.Command == CommandHelp || p.options.Command == CommandVersion {
		return nil
	}
	if p.options.Command == CommandAsk {
		p.detectCommand()
	}
	if err := p.validateSubcommand(); err != nil {
		return err
	}
	return nil
}

func (p *parser) takeValue(args []string, index *int, flag, inline string, hasInline bool) (string, error) {
	if hasInline {
		if inline == "" {
			return "", failure.New(
				failure.KindUsage,
				"usage.value_required",
				map[string]string{"flag": flag},
			)
		}
		return inline, nil
	}
	if *index+1 >= len(args) {
		return "", failure.New(
			failure.KindUsage,
			"usage.value_required",
			map[string]string{"flag": flag},
		)
	}
	*index++
	return args[*index], nil
}

func (p *parser) validateConflicts() error {
	conflicts := [][2]string{
		{"thinking", "no-thinking"},
		{"show-thinking", "hide-thinking"},
		{"no-thinking", "show-thinking"},
		{"new", "conversation"},
		{"markdown", "no-markdown"},
		{"markdown", "render-mode-plain"},
	}
	if p.options.RenderMode == "plain" && p.seen["markdown"] {
		p.seen["render-mode-plain"] = true
	}
	for _, pair := range conflicts {
		if p.seen[pair[0]] && p.seen[pair[1]] {
			left := "--" + pair[0]
			right := "--" + pair[1]
			if pair[1] == "render-mode-plain" {
				right = "--render-mode plain"
			}
			return failure.New(
				failure.KindUsage,
				"usage.conflict",
				map[string]string{"left": left, "right": right},
			)
		}
	}
	return nil
}

func (p *parser) detectCommand() {
	if p.afterSeparator || len(p.positionals) == 0 {
		return
	}
	switch p.positionals[0] {
	case "config":
		p.options.Command = CommandConfig
		p.positionals = p.positionals[1:]
	case "conversations":
		p.options.Command = CommandConversations
		p.positionals = p.positionals[1:]
	}
}

func (p *parser) validateSubcommand() error {
	switch p.options.Command {
	case CommandConfig:
		if len(p.positionals) != 0 {
			return failure.New(
				failure.KindUsage,
				"usage.unknown_command",
				map[string]string{"command": strings.Join(append([]string{"config"}, p.positionals...), " ")},
			)
		}
	case CommandConversations:
		if len(p.positionals) == 0 {
			p.options.ConversationsAction = "select"
			return nil
		}
		action := p.positionals[0]
		switch action {
		case "list", "select":
			p.options.ConversationsAction = action
		case "delete", "rename":
			p.options.ConversationsAction = action
			if len(p.positionals) > 1 {
				p.options.ConversationsID = p.positionals[1]
			}
		default:
			return failure.New(
				failure.KindUsage,
				"usage.unknown_command",
				map[string]string{"command": "conversations " + strings.Join(p.positionals, " ")},
			)
		}
		if len(p.positionals) > 2 {
			return failure.New(
				failure.KindUsage,
				"usage.unknown_command",
				map[string]string{"command": "conversations " + strings.Join(p.positionals, " ")},
			)
		}
	}
	return nil
}

func splitFlag(token string) (name, value string, hasValue bool) {
	if !strings.HasPrefix(token, "-") || token == "-" {
		return token, "", false
	}
	if strings.HasPrefix(token, "--") {
		if index := strings.IndexByte(token, '='); index >= 0 {
			return token[:index], token[index+1:], true
		}
	}
	return token, "", false
}

func invalidValue(flag, value, expected string) error {
	return failure.New(
		failure.KindUsage,
		"usage.invalid_value",
		map[string]string{
			"flag":     flag,
			"value":    value,
			"expected": expected,
		},
	)
}

func buildCorrection(first string, remaining []string) string {
	quoted := `"` + "-- " + first + `"`
	parts := append([]string{"ask", quoted}, remaining...)
	return strings.Join(parts, " ")
}

func (o Options) String() string {
	return fmt.Sprintf("command=%s question=%q provider=%q model=%q", o.Command, o.Question, o.ProviderOverride, o.ModelOverride)
}
