package ask

import (
	"testing"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestShouldEnableThinkingUsesMultilingualLexicon(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{name: "chinese greeting", prompt: "你好", want: false},
		{name: "english greeting", prompt: "hello?", want: false},
		{name: "japanese reasoning", prompt: "なぜこのコードはデッドロックする？", want: true},
		{name: "korean reasoning", prompt: "왜 메모리 누수가 발생해?", want: true},
		{name: "english reasoning", prompt: "Why does this goroutine deadlock and how to fix it?", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldEnableThinking(test.prompt, 0); got != test.want {
				t.Fatalf("shouldEnableThinking() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestShouldEnableThinkingUsesPipeSize(t *testing.T) {
	if !shouldEnableThinking("summarize this stack trace", 501) {
		t.Fatal("shouldEnableThinking() = false, want true")
	}
}

func TestAutoThinkingRequestUsesHeuristicSwitch(t *testing.T) {
	config := settings.Default().Reasoning
	profile := settings.Provider{Format: string(provider.FormatOpenAIChatCompletions)}
	route := provider.Route{Provider: "opencode", Model: "deepseek-v4.1-flash"}

	low := autoThinkingRequest(config, route, profile, "hello", 0)
	if !low.Explicit || low.Enabled || low.Value != "none" || low.OmitWhenOff {
		t.Fatalf("low thinking = %#v", low)
	}
	enabled := autoThinkingRequest(config, route, profile, "Why does this deadlock?", 0)
	if !enabled.Explicit || !enabled.Enabled || enabled.Value != "low" {
		t.Fatalf("enabled thinking = %#v", enabled)
	}
}

func TestAutoThinkingRequestHonorsDisabledBehavior(t *testing.T) {
	config := settings.Default().Reasoning
	config.DisabledBehavior = "omit"
	profile := settings.Provider{Format: string(provider.FormatOpenAIChatCompletions)}
	route := provider.Route{Provider: "opencode", Model: "deepseek-v4.1-flash"}

	thinking := autoThinkingRequest(config, route, profile, "hello", 0)
	if !thinking.Explicit || thinking.Enabled || !thinking.OmitWhenOff {
		t.Fatalf("disabled thinking = %#v", thinking)
	}
}

func TestAutoThinkingRequestOmitsUnknownModel(t *testing.T) {
	config := settings.Default().Reasoning
	profile := settings.Provider{Format: string(provider.FormatOpenAIChatCompletions)}
	route := provider.Route{Provider: "custom", Model: "unknown-model"}
	thinking := autoThinkingRequest(config, route, profile, "Why?", 0)
	if thinking.Explicit {
		t.Fatalf("unknown model thinking = %#v", thinking)
	}
}
