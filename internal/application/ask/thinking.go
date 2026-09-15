package ask

import (
	"context"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func resolveThinking(
	ctx context.Context,
	config settings.Reasoning,
	route provider.Route,
	profile settings.Provider,
	override *provider.ThinkingMode,
	question string,
	stdinCharacters int,
	resolver ports.ModelCapabilityResolver,
) (provider.ThinkingRequest, error) {
	mode := provider.ThinkingMode(config.Mode)
	if override != nil {
		mode = *override
	}
	switch mode {
	case provider.ThinkingAuto:
		return autoThinkingRequest(config, route, profile, question, stdinCharacters), nil
	case provider.ThinkingOff:
		return disabledThinkingRequest(config), nil
	case provider.ThinkingOn:
		value, anthropicBudget, err := minimumReasoningValue(route.Model, profile.Format, config)
		if err != nil && failure.Info(err).MessageID == "reasoning.model_unknown" && resolver != nil {
			resolved, ok, lookupErr := resolver.MinimumReasoningEffort(ctx, route, profile)
			if lookupErr == nil && ok {
				value = resolved
				err = nil
			}
		}
		if err != nil {
			return provider.ThinkingRequest{}, err
		}
		return provider.ThinkingRequest{
			Explicit:     true,
			Enabled:      true,
			Field:        config.RequestField,
			Value:        value,
			AnthropicMax: anthropicBudget,
		}, nil
	default:
		return provider.ThinkingRequest{}, failure.New(
			failure.KindConfig,
			"config.reasoning_mode_invalid",
			map[string]string{"value": string(mode)},
		)
	}
}

func autoThinkingRequest(
	config settings.Reasoning,
	route provider.Route,
	profile settings.Provider,
	question string,
	stdinCharacters int,
) provider.ThinkingRequest {
	value, budget, err := minimumReasoningValue(route.Model, profile.Format, config)
	if err != nil {
		return provider.ThinkingRequest{OmitWhenOff: true}
	}
	if !shouldEnableThinking(question, stdinCharacters) {
		return disabledThinkingRequest(config)
	}
	if profile.Format == string(provider.FormatAnthropicMessages) {
		return provider.ThinkingRequest{
			Explicit:     true,
			Enabled:      true,
			AnthropicMax: budget,
		}
	}
	return provider.ThinkingRequest{
		Explicit: true,
		Enabled:  true,
		Field:    config.RequestField,
		Value:    value,
	}
}

func disabledThinkingRequest(config settings.Reasoning) provider.ThinkingRequest {
	return provider.ThinkingRequest{
		Explicit:    true,
		Enabled:     false,
		Field:       config.RequestField,
		Value:       config.DisabledValue,
		OmitWhenOff: config.DisabledBehavior == "omit",
	}
}

func minimumReasoningValue(model, format string, config settings.Reasoning) (string, int, error) {
	if config.EnabledValue != "" && config.EnabledValue != "minimum" {
		return config.EnabledValue, 0, nil
	}
	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "gpt-5"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "o4"):
		if format == string(provider.FormatAnthropicMessages) {
			return "enabled", 1024, nil
		}
		return "minimal", 0, nil
	case strings.HasPrefix(lower, "claude"):
		if format == string(provider.FormatAnthropicMessages) {
			return "enabled", 1024, nil
		}
		return "minimal", 0, nil
	case strings.HasPrefix(lower, "deepseek"):
		if format == string(provider.FormatAnthropicMessages) {
			return "enabled", 1024, nil
		}
		return "low", 0, nil
	default:
		return "", 0, failure.New(
			failure.KindConfig,
			"reasoning.model_unknown",
			map[string]string{"model": model},
		)
	}
}
