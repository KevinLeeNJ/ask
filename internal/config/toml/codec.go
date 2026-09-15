package toml

import (
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func (d providerDTO) toDomain() (settings.Provider, error) {
	timeout, err := parseDuration(d.RequestTimeout)
	if err != nil {
		return settings.Provider{}, failure.Wrap(
			failure.KindConfig,
			"config.request_timeout_invalid",
			map[string]string{"value": d.RequestTimeout},
			err,
		)
	}
	return settings.Provider{
		Format:         d.Format,
		BaseURL:        d.BaseURL,
		DefaultModel:   d.DefaultModel,
		EnabledModels:  append([]string(nil), d.EnabledModels...),
		RequestTimeout: timeout,
		Headers:        cloneHeaders(d.Headers),
	}, nil
}

func decodeProviders(raw map[string]providerDTO) (map[string]settings.Provider, error) {
	providers := make(map[string]settings.Provider, len(raw))
	for id, dto := range raw {
		profile, err := dto.toDomain()
		if err != nil {
			return nil, err
		}
		providers[id] = profile
	}
	return providers, nil
}

func (d shellEnvDTO) toDomain() settings.ShellEnv {
	return settings.ShellEnv{
		Shell:        d.Shell,
		Syntax:       d.Syntax,
		ProfileFile:  d.ProfileFile,
		ManagedBlock: d.ManagedBlock,
	}
}

func (d historyDTO) toDomain() settings.History {
	return settings.History{Retention: d.Retention}
}

func (d chatDTO) toDomain() settings.Chat {
	return settings.Chat{
		SystemPrompt:       d.SystemPrompt,
		MaxContextMessages: d.MaxContextMessages,
		MaxContextTokens:   d.MaxContextTokens,
		Stream:             d.Stream,
	}
}

func (d reasoningDTO) toDomain() settings.Reasoning {
	return settings.Reasoning{
		Mode:                     d.Mode,
		Strategy:                 d.Strategy,
		RequestField:             d.RequestField,
		EnabledValue:             d.EnabledValue,
		DisabledValue:            d.DisabledValue,
		DisabledBehavior:         d.DisabledBehavior,
		Show:                     d.Show,
		CollapseWhenAnswerStarts: d.CollapseWhenAnswerStarts,
		AnthropicBudgetTokens:    d.AnthropicBudgetTokens,
	}
}

func (d uiDTO) toDomain() settings.UI {
	return settings.UI{
		Language:        d.Language,
		RenderMode:      d.RenderMode,
		Color:           d.Color,
		Markdown:        d.Markdown,
		MarkdownTheme:   d.MarkdownTheme,
		SyntaxHighlight: d.SyntaxHighlight,
		Hyperlinks:      d.Hyperlinks,
		ShowUsage:       d.ShowUsage,
	}
}

func (d modelDTO) toDomain() settings.Model {
	return settings.Model{
		AutoDiscover:    d.AutoDiscover,
		Temperature:     d.Temperature,
		MaxOutputTokens: d.MaxOutputTokens,
	}
}

func configFromDTO(d configDTO) (settings.Config, error) {
	providers, err := decodeProviders(d.Providers)
	if err != nil {
		return settings.Config{}, err
	}
	return settings.Config{
		Version:        d.Version,
		ActiveProvider: d.ActiveProvider,
		Model:          d.Model.toDomain(),
		Providers:      providers,
		ShellEnv:       d.ShellEnv.toDomain(),
		History:        d.History.toDomain(),
		Chat:           d.Chat.toDomain(),
		Reasoning:      d.Reasoning.toDomain(),
		UI:             d.UI.toDomain(),
	}, nil
}
