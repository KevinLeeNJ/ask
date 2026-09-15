package validation

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

var (
	providerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

var knownBaseSuffixes = []string{
	"/v1/chat/completions",
	"/v1/responses",
	"/v1/messages",
	"/v1",
}

func Normalize(config settings.Config) (settings.Config, error) {
	config.Providers = cloneProviders(config.Providers)
	if config.Providers == nil {
		config.Providers = map[string]settings.Provider{}
	}

	for id, profile := range config.Providers {
		normalized, err := NormalizeBaseURL(profile.BaseURL)
		if err != nil {
			return settings.Config{}, failure.Wrap(
				failure.KindConfig,
				"config.provider_base_url_invalid",
				map[string]string{
					"provider": id,
					"url":      profile.BaseURL,
				},
				err,
			)
		}
		profile.BaseURL = normalized
		config.Providers[id] = profile
	}
	return config, nil
}

func Validate(config settings.Config) error {
	if config.Version != 1 {
		return failure.New(
			failure.KindConfig,
			"config.version_unsupported",
			map[string]string{"version": fmt.Sprintf("%d", config.Version)},
		)
	}

	if len(config.Providers) > 0 {
		if config.ActiveProvider == "" {
			return failure.New(failure.KindConfig, "config.active_provider_missing", nil)
		}
		if _, ok := config.Providers[config.ActiveProvider]; !ok {
			return failure.New(
				failure.KindConfig,
				"config.active_provider_unknown",
				map[string]string{"provider": config.ActiveProvider},
			)
		}
	}

	for id, profile := range config.Providers {
		if !providerIDPattern.MatchString(id) {
			return failure.New(
				failure.KindConfig,
				"config.provider_id_invalid",
				map[string]string{"provider": id},
			)
		}
		if err := validateProvider(id, profile); err != nil {
			return err
		}
	}

	switch config.History.Retention {
	case settings.Retention7Days, settings.Retention30Days, settings.Retention60Days, settings.RetentionNever:
	default:
		return failure.New(
			failure.KindConfig,
			"config.retention_invalid",
			map[string]string{"value": config.History.Retention},
		)
	}
	if config.Chat.MaxContextMessages <= 0 {
		return failure.New(failure.KindConfig, "config.context_messages_invalid", nil)
	}
	if config.Chat.MaxContextTokens <= 0 {
		return failure.New(failure.KindConfig, "config.context_tokens_invalid", nil)
	}
	if config.Model.MaxOutputTokens <= 0 {
		return failure.New(failure.KindConfig, "config.max_output_tokens_invalid", nil)
	}
	if config.Model.Temperature < 0 || config.Model.Temperature > 2 {
		return failure.New(failure.KindConfig, "config.temperature_invalid", nil)
	}

	if !validReasoningMode(settings.ReasoningMode(config.Reasoning.Mode)) {
		return failure.New(
			failure.KindConfig,
			"config.reasoning_mode_invalid",
			map[string]string{"value": config.Reasoning.Mode},
		)
	}
	if config.Reasoning.DisabledBehavior != "send" && config.Reasoning.DisabledBehavior != "omit" {
		return failure.New(
			failure.KindConfig,
			"config.reasoning_disabled_behavior_invalid",
			map[string]string{"value": config.Reasoning.DisabledBehavior},
		)
	}

	if err := validateUI(config.UI); err != nil {
		return err
	}
	return nil
}

func validReasoningMode(m settings.ReasoningMode) bool {
	switch m {
	case settings.ReasoningAuto, settings.ReasoningOn, settings.ReasoningOff:
		return true
	default:
		return false
	}
}

func validateProvider(id string, profile settings.Provider) error {
	if strings.TrimSpace(profile.BaseURL) == "" {
		return failure.New(
			failure.KindConfig,
			"config.base_url_missing",
			map[string]string{"provider": id},
		)
	}
	if strings.TrimSpace(profile.DefaultModel) == "" {
		return failure.New(
			failure.KindConfig,
			"config.default_model_missing",
			map[string]string{"provider": id},
		)
	}
	if len(profile.EnabledModels) == 0 {
		return failure.New(
			failure.KindConfig,
			"config.enabled_models_missing",
			map[string]string{"provider": id},
		)
	}

	foundDefault := false
	seen := map[string]struct{}{}
	for _, model := range profile.EnabledModels {
		model = strings.TrimSpace(model)
		if model == "" {
			return failure.New(
				failure.KindConfig,
				"config.enabled_model_empty",
				map[string]string{"provider": id},
			)
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		if model == profile.DefaultModel {
			foundDefault = true
		}
	}
	if !foundDefault {
		return failure.New(
			failure.KindConfig,
			"config.default_model_not_enabled",
			map[string]string{
				"provider": id,
				"model":    profile.DefaultModel,
			},
		)
	}

	format := provider.Format(profile.Format)
	if !format.Valid() {
		return failure.New(
			failure.KindConfig,
			"config.provider_format_invalid",
			map[string]string{"provider": id, "value": profile.Format},
		)
	}

	if profile.RequestTimeout <= 0 {
		return failure.New(
			failure.KindConfig,
			"config.request_timeout_invalid",
			map[string]string{"provider": id},
		)
	}
	for name, value := range profile.Headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			return failure.New(
				failure.KindConfig,
				"config.header_name_invalid",
				map[string]string{"provider": id},
			)
		}
		if lower == "authorization" ||
			lower == "x-api-key" ||
			lower == "api-key" ||
			lower == "content-type" ||
			lower == "anthropic-version" ||
			lower == "user-agent" {
			return failure.New(
				failure.KindConfig,
				"config.header_reserved",
				map[string]string{"provider": id, "header": name},
			)
		}
		if strings.ContainsAny(value, "\r\n") {
			return failure.New(
				failure.KindConfig,
				"config.header_value_invalid",
				map[string]string{"provider": id, "header": name},
			)
		}
	}
	return nil
}

func validateUI(ui settings.UI) error {
	switch ui.Language {
	case "auto", "zh-CN", "en-US":
	default:
		return failure.New(
			failure.KindConfig,
			"config.ui_language_invalid",
			map[string]string{"value": ui.Language},
		)
	}
	switch ui.RenderMode {
	case "auto", "full", "inline", "plain":
	default:
		return failure.New(
			failure.KindConfig,
			"config.render_mode_invalid",
			map[string]string{"value": ui.RenderMode},
		)
	}
	switch ui.Markdown {
	case "auto", "on", "off":
	default:
		return failure.New(
			failure.KindConfig,
			"config.ui_markdown_invalid",
			map[string]string{"value": ui.Markdown},
		)
	}
	switch ui.Color {
	case "auto", "on", "off":
	default:
		return failure.New(
			failure.KindConfig,
			"config.ui_color_invalid",
			map[string]string{"value": ui.Color},
		)
	}
	return nil
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", failure.New(failure.KindConfig, "config.base_url_empty", nil)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", failure.Wrap(
			failure.KindConfig,
			"config.base_url_unparseable",
			nil,
			err,
		)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", failure.New(failure.KindConfig, "config.base_url_scheme_invalid", nil)
	}
	if parsed.Host == "" {
		return "", failure.New(failure.KindConfig, "config.base_url_host_missing", nil)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", failure.New(failure.KindConfig, "config.base_url_query_fragment", nil)
	}

	cleanPath := path.Clean(parsed.EscapedPath())
	if cleanPath == "." || cleanPath == "/" {
		cleanPath = ""
	}
	for _, suffix := range knownBaseSuffixes {
		if cleanPath == suffix {
			cleanPath = ""
			break
		}
		if strings.HasSuffix(cleanPath, suffix) {
			cleanPath = strings.TrimSuffix(cleanPath, suffix)
			break
		}
	}
	cleanPath = strings.TrimSuffix(cleanPath, "/")
	parsed.Path = cleanPath
	parsed.RawPath = ""
	parsed.ForceQuery = false
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func cloneProviders(input map[string]settings.Provider) map[string]settings.Provider {
	if input == nil {
		return nil
	}
	output := make(map[string]settings.Provider, len(input))
	for id, profile := range input {
		output[id] = profile
	}
	return output
}
