package flags

import (
	"strings"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

func TestParseQuestionArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "zero", args: nil, want: ""},
		{name: "one", args: []string{"hello"}, want: "hello"},
		{name: "many", args: []string{"你是什么模型", "你的架构是什么"}, want: "你是什么模型 你的架构是什么"},
		{name: "flag mixed", args: []string{"--no-thinking", "分析这段代码", "的风险"}, want: "分析这段代码 的风险"},
		{name: "unknown dash token", args: []string{"-foo", "bar"}, want: "-foo bar"},
		{name: "quoted separator text", args: []string{"-- --model", "是什么"}, want: "-- --model 是什么"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options, err := Parse(test.args)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if options.Question != test.want {
				t.Fatalf("Question = %q, want %q", options.Question, test.want)
			}
		})
	}
}

func TestParseReservedCommands(t *testing.T) {
	configOptions, err := Parse([]string{"config"})
	if err != nil || configOptions.Command != CommandConfig {
		t.Fatalf("config parse = %#v, %v", configOptions, err)
	}
	quotedOptions, err := Parse([]string{"config"})
	if err != nil || quotedOptions.Command != CommandConfig {
		t.Fatalf("same argv for quoted config differs: %#v, %v", quotedOptions, err)
	}
	literalOptions, err := Parse([]string{"--", "config"})
	if err != nil {
		t.Fatalf("literal config parse error = %v", err)
	}
	if literalOptions.Command != CommandAsk || literalOptions.Question != "config" {
		t.Fatalf("literal config parse = %#v", literalOptions)
	}
	conversationOptions, err := Parse([]string{"conversations", "list"})
	if err != nil {
		t.Fatalf("conversations parse error = %v", err)
	}
	if conversationOptions.Command != CommandConversations || conversationOptions.ConversationsAction != "list" {
		t.Fatalf("conversations parse = %#v", conversationOptions)
	}
}

func TestParseSeparatorOptionError(t *testing.T) {
	_, err := Parse([]string{"--", "--model", "是什么"})
	if err == nil {
		t.Fatal("expected separator error")
	}
	if !strings.Contains(err.Error(), "usage.separator_option") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseConflicts(t *testing.T) {
	tests := [][]string{
		{"--thinking", "--no-thinking", "x"},
		{"--show-thinking", "--hide-thinking", "x"},
		{"--no-thinking", "--show-thinking", "x"},
		{"--new", "--conversation", "id", "x"},
		{"--markdown", "--no-markdown", "x"},
		{"--markdown", "--render-mode", "plain", "x"},
	}
	for _, args := range tests {
		if _, err := Parse(args); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", args)
		}
	}
}

func TestParseOverrides(t *testing.T) {
	options, err := Parse([]string{
		"--lang", "zh-CN",
		"--provider=work",
		"--model", "model-a",
		"--system", "system",
		"--timeout", "3s",
		"--max-context-messages", "12",
		"--json",
		"--quiet",
		"hello",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if options.LanguageOverride != "zh-CN" ||
		options.ProviderOverride != "work" ||
		options.ModelOverride != "model-a" ||
		options.SystemOverride == nil || *options.SystemOverride != "system" ||
		options.TimeoutOverride == nil || *options.TimeoutOverride != 3*time.Second ||
		options.MaxContextMessages != 12 ||
		!options.JSON ||
		!options.Quiet {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestThinkingOverride(t *testing.T) {
	options, err := Parse([]string{"--thinking", "question"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if options.ThinkingOverride == nil || *options.ThinkingOverride != provider.ThinkingOn {
		t.Fatalf("ThinkingOverride = %#v", options.ThinkingOverride)
	}
}
