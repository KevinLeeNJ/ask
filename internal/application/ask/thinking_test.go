package ask

import (
	"context"
	"errors"
	"testing"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestResolveThinkingUsesCapabilityResolverForUnknownModel(t *testing.T) {
	resolver := staticCapabilityResolver{value: "low", ok: true}
	thinking, err := resolveThinking(
		context.Background(),
		settings.Default().Reasoning,
		provider.Route{Provider: "custom", Model: "mystery-model"},
		settings.Provider{Format: string(provider.FormatOpenAIChatCompletions), BaseURL: "https://example.com"},
		pointer(provider.ThinkingOn),
		"question",
		0,
		resolver,
	)
	if err != nil {
		t.Fatalf("resolveThinking() error = %v", err)
	}
	if !thinking.Explicit || !thinking.Enabled || thinking.Value != "low" {
		t.Fatalf("thinking = %#v", thinking)
	}
}

func TestResolveThinkingKeepsUnknownErrorWhenLookupFails(t *testing.T) {
	resolver := staticCapabilityResolver{err: errors.New("offline")}
	_, err := resolveThinking(
		context.Background(),
		settings.Default().Reasoning,
		provider.Route{Provider: "custom", Model: "mystery-model"},
		settings.Provider{Format: string(provider.FormatOpenAIChatCompletions), BaseURL: "https://example.com"},
		pointer(provider.ThinkingOn),
		"question",
		0,
		resolver,
	)
	if failure.Info(err).MessageID != "reasoning.model_unknown" {
		t.Fatalf("error = %v", err)
	}
}

type staticCapabilityResolver struct {
	value string
	ok    bool
	err   error
}

func (r staticCapabilityResolver) MinimumReasoningEffort(
	context.Context,
	provider.Route,
	settings.Provider,
) (string, bool, error) {
	return r.value, r.ok, r.err
}

func pointer[T any](value T) *T {
	return &value
}
