package toml

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	burnttoml "github.com/BurntSushi/toml"

	"github.com/KevinLeeNJ/ask/internal/config/validation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/platform/configdir"
	"github.com/KevinLeeNJ/ask/internal/platform/filesystem"
)

const configFileName = "config.toml"

type Store struct {
	Path string
}

func NewStore(path string) *Store {
	return &Store{Path: path}
}

func DefaultPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("ASK_CONFIG_FILE")); override != "" {
		return filepath.Abs(override)
	}
	configDir, err := configdir.AskDir()
	if err != nil {
		return "", failure.Wrap(failure.KindConfig, "config.user_dir_unavailable", nil, err)
	}
	return filepath.Join(configDir, configFileName), nil
}

func (s *Store) Load(ctx context.Context) (settings.Config, error) {
	select {
	case <-ctx.Done():
		return settings.Config{}, ctx.Err()
	default:
	}

	raw := map[string]any{}
	if _, err := burnttoml.DecodeFile(s.Path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings.Config{}, failure.Wrap(
				failure.KindConfig,
				"config.not_found",
				map[string]string{"path": s.Path},
				err,
			)
		}
		return settings.Config{}, failure.Wrap(
			failure.KindConfig,
			"config.read_failed",
			map[string]string{"path": s.Path, "reason": err.Error()},
			err,
		)
	}
	if err := rejectSecretFields(raw, ""); err != nil {
		return settings.Config{}, err
	}

	dto := toDTO(settings.Default())
	if _, err := burnttoml.DecodeFile(s.Path, &dto); err != nil {
		return settings.Config{}, failure.Wrap(
			failure.KindConfig,
			"config.decode_failed",
			map[string]string{"path": s.Path, "reason": err.Error()},
			err,
		)
	}
	config, err := configFromDTO(dto)
	if err != nil {
		return settings.Config{}, err
	}
	config, err = validation.Normalize(config)
	if err != nil {
		return settings.Config{}, err
	}
	if err := validation.Validate(config); err != nil {
		return settings.Config{}, err
	}
	return config, nil
}

func (s *Store) Save(ctx context.Context, config settings.Config) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	normalized, err := validation.Normalize(config)
	if err != nil {
		return err
	}
	if err := validation.Validate(normalized); err != nil {
		return err
	}

	var buffer bytes.Buffer
	if err := burnttoml.NewEncoder(&buffer).Encode(toDTO(normalized)); err != nil {
		return failure.Wrap(failure.KindConfig, "config.encode_failed", nil, err)
	}
	if err := filesystem.AtomicWriteFile(s.Path, buffer.Bytes(), 0o600, 0o700); err != nil {
		return failure.Wrap(
			failure.KindConfig,
			"config.write_failed",
			map[string]string{"path": s.Path, "reason": err.Error()},
			err,
		)
	}
	return nil
}

func rejectSecretFields(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			lower := strings.ToLower(key)
			if lower == "api_key" || lower == "apikey" || lower == "secret" || lower == "password" || lower == "token" || strings.HasSuffix(lower, "_secret") || strings.HasSuffix(lower, "_password") || strings.HasSuffix(lower, "_api_key") {
				return failure.New(
					failure.KindConfig,
					"config.secret_field_forbidden",
					map[string]string{"field": childPath},
				)
			}
			if err := rejectSecretFields(child, childPath); err != nil {
				return err
			}
		}
	case []map[string]any:
		for index, child := range typed {
			if err := rejectSecretFields(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectSecretFields(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

type configDTO struct {
	Version        int                    `toml:"version"`
	ActiveProvider string                 `toml:"active_provider"`
	Model          modelDTO               `toml:"model"`
	Providers      map[string]providerDTO `toml:"providers"`
	ShellEnv       shellEnvDTO            `toml:"shell_env"`
	History        historyDTO             `toml:"history"`
	Chat           chatDTO                `toml:"chat"`
	Reasoning      reasoningDTO           `toml:"reasoning"`
	UI             uiDTO                  `toml:"ui"`
}

type modelDTO struct {
	AutoDiscover    bool    `toml:"auto_discover"`
	Temperature     float64 `toml:"temperature"`
	MaxOutputTokens int     `toml:"max_output_tokens"`
}

type providerDTO struct {
	Format         string            `toml:"format"`
	BaseURL        string            `toml:"base_url"`
	DefaultModel   string            `toml:"default_model"`
	EnabledModels  []string          `toml:"enabled_models"`
	RequestTimeout string            `toml:"request_timeout"`
	Headers        map[string]string `toml:"headers,omitempty"`
}

type shellEnvDTO struct {
	Shell        string `toml:"shell"`
	Syntax       string `toml:"syntax"`
	ProfileFile  string `toml:"profile_file"`
	ManagedBlock bool   `toml:"managed_block"`
}

type historyDTO struct {
	Retention string `toml:"retention"`
}

type chatDTO struct {
	SystemPrompt       string `toml:"system_prompt"`
	MaxContextMessages int    `toml:"max_context_messages"`
	MaxContextTokens   int    `toml:"max_context_tokens"`
	Stream             bool   `toml:"stream"`
}

type reasoningDTO struct {
	Mode                     string `toml:"mode"`
	Strategy                 string `toml:"strategy"`
	RequestField             string `toml:"request_field"`
	EnabledValue             string `toml:"enabled_value"`
	DisabledValue            string `toml:"disabled_value"`
	DisabledBehavior         string `toml:"disabled_behavior"`
	Show                     string `toml:"show"`
	CollapseWhenAnswerStarts bool   `toml:"collapse_when_answer_starts"`
	AnthropicBudgetTokens    string `toml:"anthropic_budget_tokens"`
}

type uiDTO struct {
	Language        string `toml:"language"`
	RenderMode      string `toml:"render_mode"`
	Color           string `toml:"color"`
	Markdown        string `toml:"markdown"`
	MarkdownTheme   string `toml:"markdown_theme"`
	SyntaxHighlight bool   `toml:"syntax_highlight"`
	Hyperlinks      string `toml:"hyperlinks"`
	ShowUsage       bool   `toml:"show_usage"`
}

func toDTO(config settings.Config) configDTO {
	providers := make(map[string]providerDTO, len(config.Providers))
	for id, profile := range config.Providers {
		providers[id] = providerDTO{
			Format:         profile.Format,
			BaseURL:        profile.BaseURL,
			DefaultModel:   profile.DefaultModel,
			EnabledModels:  append([]string(nil), profile.EnabledModels...),
			RequestTimeout: profile.RequestTimeout.String(),
			Headers:        cloneHeaders(profile.Headers),
		}
	}
	return configDTO{
		Version:        config.Version,
		ActiveProvider: config.ActiveProvider,
		Model: modelDTO{
			AutoDiscover:    config.Model.AutoDiscover,
			Temperature:     config.Model.Temperature,
			MaxOutputTokens: config.Model.MaxOutputTokens,
		},
		Providers: providers,
		ShellEnv: shellEnvDTO{
			Shell:        config.ShellEnv.Shell,
			Syntax:       config.ShellEnv.Syntax,
			ProfileFile:  config.ShellEnv.ProfileFile,
			ManagedBlock: config.ShellEnv.ManagedBlock,
		},
		History: historyDTO{Retention: config.History.Retention},
		Chat: chatDTO{
			SystemPrompt:       config.Chat.SystemPrompt,
			MaxContextMessages: config.Chat.MaxContextMessages,
			MaxContextTokens:   config.Chat.MaxContextTokens,
			Stream:             config.Chat.Stream,
		},
		Reasoning: reasoningDTO{
			Mode:                     config.Reasoning.Mode,
			Strategy:                 config.Reasoning.Strategy,
			RequestField:             config.Reasoning.RequestField,
			EnabledValue:             config.Reasoning.EnabledValue,
			DisabledValue:            config.Reasoning.DisabledValue,
			DisabledBehavior:         config.Reasoning.DisabledBehavior,
			Show:                     config.Reasoning.Show,
			CollapseWhenAnswerStarts: config.Reasoning.CollapseWhenAnswerStarts,
			AnthropicBudgetTokens:    config.Reasoning.AnthropicBudgetTokens,
		},
		UI: uiDTO{
			Language:        config.UI.Language,
			RenderMode:      config.UI.RenderMode,
			Color:           config.UI.Color,
			Markdown:        config.UI.Markdown,
			MarkdownTheme:   config.UI.MarkdownTheme,
			SyntaxHighlight: config.UI.SyntaxHighlight,
			Hyperlinks:      config.UI.Hyperlinks,
			ShowUsage:       config.UI.ShowUsage,
		},
	}
}

func cloneHeaders(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func parseDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return duration, nil
}
