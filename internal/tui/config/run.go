package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"github.com/KevinLeeNJ/ask/internal/application/configure"
	"github.com/KevinLeeNJ/ask/internal/config/validation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
	"github.com/KevinLeeNJ/ask/internal/tui/widgets"
)

const (
	actionProviders    = "providers"
	actionDefaultModel = "default-model"
	actionSystem       = "system"
	actionContext      = "context"
	actionThinking     = "thinking"
	actionDisplay      = "display"
	actionSave         = "save"
	actionSettingsBack = "__settings_back__"
	contextMessages    = "context-messages"
	contextTokens      = "context-tokens"
	contextStream      = "context-stream"
	contextRetention   = "context-retention"
	thinkingMode       = "thinking-mode"
	thinkingShow       = "thinking-show"
	displayLanguage    = "display-language"
	displayRenderMode  = "display-render-mode"
	displayMarkdown    = "display-markdown"
	displayColor       = "display-color"
	displayHyperlinks  = "display-hyperlinks"
	providerActionAdd  = "__provider_add__"
	providerActionBack = "__provider_back__"
)

var providerColumnWidths = []int{20, 24, 28}

const providerCompactWidth = 80

var errSubmenuBack = errors.New("submenu back requested")

type state struct {
	initial  settings.Config
	config   settings.Config
	secrets  map[string]configure.SecretChange
	removals map[string]settings.Provider
}

func Run(
	ctx context.Context,
	service *configure.Service,
	initial settings.Config,
	translator *catalog.Catalog,
) error {
	if service == nil {
		return failure.New(failure.KindInternal, "config.tui_not_available", nil)
	}
	initialConfig := cloneConfig(initial)
	current := &state{
		initial:  initialConfig,
		config:   cloneConfig(initialConfig),
		secrets:  map[string]configure.SecretChange{},
		removals: map[string]settings.Provider{},
	}
	action := actionProviders
	for {
		form := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(translator.Text("menu.config.title", nil)).
				Description(menuDescription(current.config, translator)).
				Options(rootMenuOptions(translator, current.providerDirty())...).
				Value(&action),
		)).WithTheme(huh.ThemeCharm()).WithAccessible(false)
		if err := runRootForm(ctx, translator, form); err != nil {
			if errors.Is(err, errExitApp) {
				return nil
			}
			if errors.Is(err, errCancelled) {
				if !current.dirty() {
					return nil
				}
				save := false
				if err := runForm(ctx, translator, huh.NewConfirm().
					Title(translator.Text("menu.config.save_title", nil)).
					Description(translator.Text("menu.config.save_description", nil)).
					Affirmative(saveActionLabel(current.providerDirty(), translator)).
					Negative(translator.Text("menu.action.discard_changes", nil)).
					Value(&save)); err != nil {
					if errors.Is(err, errCancelled) {
						return nil
					}
					return err
				}
				if !save {
					return nil
				}
				if err := saveChanges(ctx, service, current, translator); err != nil {
					fmt.Fprintln(os.Stderr, "ask:", userError(translator, err))
					continue
				}
				return nil
			}
			return err
		}

		var err error
		switch action {
		case actionProviders:
			err = manageProviders(ctx, service, current, translator)
		case actionDefaultModel:
			err = selectDefaultModel(current, translator)
		case actionSystem:
			err = editSystem(current, translator)
		case actionContext:
			err = editContext(current, translator)
		case actionThinking:
			err = editThinking(current, translator)
		case actionDisplay:
			err = editDisplay(current, translator)
		case actionSave:
			err = saveChanges(ctx, service, current, translator)
			if err == nil {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, errExitApp) {
				return nil
			}
			if errors.Is(err, errCancelled) {
				continue
			}
			fmt.Fprintln(os.Stderr, "ask:", userError(translator, err))
		}
	}
}

func manageProviders(
	ctx context.Context,
	service *configure.Service,
	current *state,
	translator *catalog.Catalog,
) error {
	selected := initialProviderSelection(current.config)
	for {
		width := terminalWidth()
		field := huh.NewSelect[string]().
			Title(translator.Text("menu.provider.title", nil)).
			Description(providerColumns(translator, width)).
			Options(providerSelectionOptions(current.config, translator, width)...).
			Value(&selected)
		deletedID, deleteRequested, err := runProviderSelection(ctx, field, translator)
		if err != nil {
			return err
		}
		if deleteRequested {
			if err := deleteProvider(current, translator, deletedID); err != nil {
				if errors.Is(err, errCancelled) {
					continue
				}
				return err
			}
			continue
		}
		switch selected {
		case providerActionAdd:
			if err := addProvider(ctx, current, service, translator); err != nil {
				if errors.Is(err, errCancelled) {
					continue
				}
				return err
			}
			continue
		case providerActionBack, "":
			return nil
		default:
			if err := editProvider(ctx, current, service, translator, selected); err != nil {
				if errors.Is(err, errCancelled) {
					continue
				}
				return err
			}
			continue
		}
	}
}

func addProvider(
	ctx context.Context,
	current *state,
	service *configure.Service,
	translator *catalog.Catalog,
) error {
	confirmed := false
	if err := runForm(ctx, translator, huh.NewConfirm().
		Title(translator.Text("menu.provider.compatible_title", nil)).
		Description(translator.Text("menu.provider.compatible_description", nil)).
		Affirmative(translator.Text("menu.action.continue", nil)).
		Negative(translator.Text("menu.action.cancel", nil)).
		Value(&confirmed)); err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	id := ""
	format := string(provider.FormatOpenAIResponses)
	baseURL := ""
	timeout := "60s"
	apiKey := ""
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(translator.Text("menu.provider.identifier", nil)).
				Value(&id).
				Validate(func(value string) error {
					return validateProviderID(value, translator)
				}),
			huh.NewSelect[string]().
				Title(translator.Text("menu.provider.api_format", nil)).
				Options(
					huh.NewOption("OpenAI Chat Completions", string(provider.FormatOpenAIChatCompletions)),
					huh.NewOption("OpenAI Responses", string(provider.FormatOpenAIResponses)),
					huh.NewOption("Anthropic Messages", string(provider.FormatAnthropicMessages)),
				).
				Value(&format),
			huh.NewInput().
				Title(translator.Text("menu.provider.base_url", nil)).
				Value(&baseURL).
				Validate(func(value string) error {
					return validateBaseURL(value, translator)
				}),
			huh.NewInput().Title(translator.Text("menu.provider.request_timeout", nil)).Value(&timeout),
		),
		huh.NewGroup(
			huh.NewInput().
				Title(translator.Text("menu.provider.api_key_hidden", nil)).
				EchoMode(huh.EchoModePassword).
				Value(&apiKey),
		),
	).WithTheme(huh.ThemeCharm())
	if err := runBuiltForm(ctx, translator, form); err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	profile, err := providerProfile(format, baseURL, timeout, translator)
	if err != nil {
		return err
	}
	temp := cloneConfig(current.config)
	temp.Providers[id] = profile
	enabledModels, defaultModel, err := configureProviderModels(
		ctx,
		service,
		temp,
		id,
		profile,
		apiKey,
		translator,
	)
	if err != nil {
		return err
	}
	profile.EnabledModels = enabledModels
	profile.DefaultModel = defaultModel
	if err := stageProviderSecret(ctx, service, current, id, profile, apiKey, translator); err != nil {
		return err
	}
	current.config.Providers[id] = profile
	if current.config.ActiveProvider == "" {
		current.config.ActiveProvider = id
	}
	return nil
}

func editProvider(
	ctx context.Context,
	current *state,
	service *configure.Service,
	translator *catalog.Catalog,
	id string,
) error {
	if id == "" {
		return nil
	}
	profile := current.config.Providers[id]
	format := profile.Format
	baseURL := profile.BaseURL
	timeout := profile.RequestTimeout.String()
	apiKey := ""
	if err := runBuiltForm(ctx, translator, huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(translator.Text("menu.provider.base_url", nil)).
				Value(&baseURL).
				Validate(func(value string) error {
					return validateBaseURL(value, translator)
				}),
			huh.NewSelect[string]().Title(translator.Text("menu.provider.api_format", nil)).
				Options(
					huh.NewOption("OpenAI Chat Completions", string(provider.FormatOpenAIChatCompletions)),
					huh.NewOption("OpenAI Responses", string(provider.FormatOpenAIResponses)),
					huh.NewOption("Anthropic Messages", string(provider.FormatAnthropicMessages)),
				).
				Value(&format),
			huh.NewInput().Title(translator.Text("menu.provider.request_timeout", nil)).Value(&timeout),
		),
		huh.NewGroup(
			huh.NewInput().
				Title(translator.Text("menu.provider.api_key_optional", nil)).
				EchoMode(huh.EchoModePassword).
				Value(&apiKey),
		),
	).WithTheme(huh.ThemeCharm())); err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	updated, err := providerProfile(format, baseURL, timeout, translator)
	if err != nil {
		return err
	}
	updated.EnabledModels = append([]string(nil), profile.EnabledModels...)
	updated.DefaultModel = profile.DefaultModel
	updated.Headers = cloneMap(profile.Headers)
	memorySecret := apiKey
	if change, ok := current.secrets[id]; ok && strings.TrimSpace(memorySecret) == "" {
		memorySecret = change.Value
	}
	temp := cloneConfig(current.config)
	temp.Providers[id] = updated
	enabledModels, defaultModel, err := configureProviderModels(
		ctx,
		service,
		temp,
		id,
		updated,
		memorySecret,
		translator,
	)
	if err != nil {
		return err
	}
	updated.EnabledModels = enabledModels
	updated.DefaultModel = defaultModel
	if err := stageProviderSecret(ctx, service, current, id, updated, apiKey, translator); err != nil {
		return err
	}
	current.config.Providers[id] = updated
	return nil
}

func providerProfile(
	format string,
	baseURL string,
	timeout string,
	translator *catalog.Catalog,
) (settings.Provider, error) {
	normalized, err := validation.NormalizeBaseURL(baseURL)
	if err != nil {
		return settings.Provider{}, err
	}
	duration, err := time.ParseDuration(strings.TrimSpace(timeout))
	if err != nil || duration <= 0 {
		return settings.Provider{}, errors.New(translator.Text("menu.provider.validation.timeout", nil))
	}
	return settings.Provider{
		Format:         format,
		BaseURL:        normalized,
		RequestTimeout: duration,
	}, nil
}

func configureProviderModels(
	ctx context.Context,
	service *configure.Service,
	config settings.Config,
	id string,
	profile settings.Provider,
	memorySecret string,
	translator *catalog.Catalog,
) ([]string, string, error) {
	discovered := append([]string(nil), profile.EnabledModels...)
	if recent, err := service.RecentRoutes(ctx, 10); err == nil {
		for _, route := range recent {
			if string(route.Provider) == id {
				discovered = append([]string{route.Model}, discovered...)
			}
		}
	}
	var models []provider.Model
	discoverErr := runLoading(
		ctx,
		translator.Text("menu.loading.discover_models", nil)+" · "+translator.Text("menu.loading.cancel", nil),
		func(workContext context.Context) error {
			var err error
			models, err = service.DiscoverModels(workContext, config, id, memorySecret)
			return err
		},
	)
	discoveryAvailable := true
	if discoverErr != nil {
		if errors.Is(discoverErr, errExitApp) {
			return nil, "", discoverErr
		}
		discoveryAvailable = false
		fmt.Fprintln(os.Stderr, translator.Text("menu.provider.discovery_failed", map[string]string{
			"error": userError(translator, discoverErr),
		}))
	} else {
		if len(models) == 0 {
			discoveryAvailable = false
		}
		for _, model := range models {
			discovered = append(discovered, model.ID)
		}
	}
	discovered = uniqueStrings(discovered)

	enabled := append([]string(nil), profile.EnabledModels...)
	if discoveryAvailable {
		options := make([]huh.Option[string], 0, len(discovered))
		for _, model := range discovered {
			options = append(options, huh.NewOption(model, model))
		}
		if err := runModelForm(ctx, translator, huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title(translator.Text("menu.provider.enabled_models", nil)).
					Options(options...).
					Filterable(true).
					Validate(func(selected []string) error {
						if len(uniqueStrings(selected)) == 0 {
							return errors.New(translator.Text("menu.provider.enabled_models_required", nil))
						}
						return nil
					}).
					Value(&enabled),
			),
		).WithTheme(huh.ThemeCharm())); err != nil {
			return nil, "", err
		}
	} else {
		manual := strings.Join(discovered, ", ")
		if err := runBuiltForm(ctx, translator, huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(translator.Text("menu.provider.enabled_model_ids", nil)).
					Validate(func(value string) error {
						if len(splitCSV(value)) == 0 {
							return errors.New(translator.Text("menu.provider.enabled_model_id_required", nil))
						}
						return nil
					}).
					Value(&manual),
			),
		).WithTheme(huh.ThemeCharm())); err != nil {
			return nil, "", err
		}
		enabled = splitCSV(manual)
	}
	enabled = uniqueStrings(enabled)
	if len(enabled) == 0 {
		return nil, "", errors.New(translator.Text("menu.provider.model_required", nil))
	}

	defaultModel := profile.DefaultModel
	if !contains(enabled, defaultModel) {
		defaultModel = enabled[0]
	}
	options := make([]huh.Option[string], 0, len(enabled))
	for _, model := range enabled {
		options = append(options, huh.NewOption(model, model))
	}
	if err := runBuiltForm(ctx, translator, huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(translator.Text("menu.provider.default_model", nil)).
				Options(options...).
				Value(&defaultModel),
		),
	).WithTheme(huh.ThemeCharm())); err != nil {
		return nil, "", err
	}
	return enabled, defaultModel, nil
}

func stageProviderSecret(
	ctx context.Context,
	service *configure.Service,
	current *state,
	id string,
	profile settings.Provider,
	apiKey string,
	translator *catalog.Catalog,
) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	temp := cloneConfig(current.config)
	temp.Providers[id] = profile
	preview, err := service.PreviewSecret(temp, id, configure.SecretChange{Value: apiKey})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, translator.Text("menu.provider.shell_preview", nil)+preview.Content)
	overwrite := false
	if preview.Conflict {
		if err := runForm(ctx, translator, huh.NewConfirm().
			Title(translator.Text("menu.provider.env_exists", nil)).
			Description(translator.Text("menu.provider.replace_existing", nil)).
			Affirmative(translator.Text("menu.action.yes", nil)).
			Negative(translator.Text("menu.action.no", nil)).
			Value(&overwrite)); err != nil {
			return err
		}
		if !overwrite {
			return nil
		}
	}
	current.secrets[id] = configure.SecretChange{Value: apiKey, Overwrite: overwrite}
	return nil
}

func deleteProvider(current *state, translator *catalog.Catalog, id string) error {
	if id == "" {
		return nil
	}
	confirmed := false
	if err := runForm(context.Background(), translator, huh.NewConfirm().
		Title(translator.Text("menu.provider.confirm_delete", nil)).
		Description(id).
		Affirmative(translator.Text("menu.action.yes", nil)).
		Negative(translator.Text("menu.action.no", nil)).
		Value(&confirmed)); err != nil || !confirmed {
		return err
	}
	profile := current.config.Providers[id]
	if current.config.ActiveProvider == id {
		remaining := providerIDs(current.config.Providers, id)
		if len(remaining) == 0 {
			current.config.ActiveProvider = ""
		} else {
			replacement, err := chooseProviderFrom(
				remaining,
				translator,
				translator.Text("menu.provider.select_replacement", nil),
			)
			if err != nil {
				return err
			}
			current.config.ActiveProvider = replacement
		}
	}
	delete(current.config.Providers, id)
	delete(current.secrets, id)
	current.removals[id] = profile
	return nil
}

func selectDefaultModel(current *state, translator *catalog.Catalog) error {
	if len(current.config.Providers) == 0 {
		return errors.New(translator.Text("menu.provider.no_provider", nil))
	}
	id := ""
	return runSubmenu(
		func() error {
			selected, err := chooseProviderValue(
				current.config,
				translator,
				translator.Text("menu.provider.select_active", nil),
				id,
			)
			if err != nil {
				return err
			}
			if selected == "" {
				return errSubmenuBack
			}
			id = selected
			return nil
		},
		func() error {
			profile := current.config.Providers[id]
			model := profile.DefaultModel
			options := make([]huh.Option[string], 0, len(profile.EnabledModels))
			for _, item := range profile.EnabledModels {
				options = append(options, huh.NewOption(item, item))
			}
			if len(options) == 0 {
				return errors.New(translator.Text("menu.provider.no_enabled_models", nil))
			}
			if err := runForm(context.Background(), translator, huh.NewSelect[string]().
				Title(translator.Text("menu.provider.default_model", nil)).
				Options(options...).
				Value(&model)); err != nil {
				return err
			}
			current.config.ActiveProvider = id
			profile.DefaultModel = model
			current.config.Providers[id] = profile
			return errSubmenuBack
		},
	)
}

func editSystem(current *state, translator *catalog.Catalog) error {
	initial := current.config.Chat.SystemPrompt
	value := initial
	for {
		err := runSystemPromptForm(context.Background(), translator, &value)
		if err == nil {
			current.config.Chat.SystemPrompt = value
			return nil
		}
		if !errors.Is(err, errCancelled) {
			return err
		}

		edited, save, err := resolveEditedValue(initial, value, func() (bool, error) {
			save := false
			err := runForm(context.Background(), translator, huh.NewConfirm().
				Title(translator.Text("menu.config.save_title", nil)).
				Description(translator.Text("menu.system_prompt.save_description", nil)).
				Affirmative(translator.Text("menu.action.save", nil)).
				Negative(translator.Text("menu.action.discard_changes", nil)).
				Value(&save))
			return save, err
		})
		if err != nil {
			if errors.Is(err, errCancelled) {
				continue
			}
			return err
		}
		if !save {
			return nil
		}
		current.config.Chat.SystemPrompt = edited
		return nil
	}
}

func resolveEditedValue(
	initial string,
	edited string,
	confirm func() (bool, error),
) (string, bool, error) {
	if edited == initial {
		return initial, false, nil
	}
	save, err := confirm()
	if err != nil {
		return edited, false, err
	}
	return edited, save, nil
}

func editContext(current *state, translator *catalog.Catalog) error {
	action := contextMessages
	return runSubmenu(
		func() error {
			return runForm(context.Background(), translator, huh.NewSelect[string]().
				Title(translator.Text("menu.config.conversation_context", nil)).
				Options(
					huh.NewOption(
						fmt.Sprintf("%s: %d", translator.Text("menu.context.max_messages", nil), current.config.Chat.MaxContextMessages),
						contextMessages,
					),
					huh.NewOption(
						fmt.Sprintf("%s: %d", translator.Text("menu.context.max_tokens", nil), current.config.Chat.MaxContextTokens),
						contextTokens,
					),
					huh.NewOption(
						translator.Text("menu.context.stream", nil)+": "+yesNo(current.config.Chat.Stream, translator),
						contextStream,
					),
					huh.NewOption(
						translator.Text("menu.context.retention", nil)+": "+retentionLabel(current.config.History.Retention, translator),
						contextRetention,
					),
					huh.NewOption(translator.Text("menu.action.back", nil), actionSettingsBack),
				).
				Value(&action))
		},
		func() error {
			switch action {
			case contextMessages:
				return editContextMessages(current, translator)
			case contextTokens:
				return editContextTokens(current, translator)
			case contextStream:
				return editContextStream(current, translator)
			case contextRetention:
				return editContextRetention(current, translator)
			case actionSettingsBack, "":
				return errSubmenuBack
			default:
				return nil
			}
		},
	)
}

func editThinking(current *state, translator *catalog.Catalog) error {
	action := thinkingMode
	return runSubmenu(
		func() error {
			return runForm(context.Background(), translator, huh.NewSelect[string]().
				Title(translator.Text("menu.config.thinking_mode", nil)).
				Options(
					huh.NewOption(
						translator.Text("menu.thinking.mode", nil)+": "+current.config.Reasoning.Mode,
						thinkingMode,
					),
					huh.NewOption(
						translator.Text("menu.thinking.show", nil)+": "+current.config.Reasoning.Show,
						thinkingShow,
					),
					huh.NewOption(translator.Text("menu.action.back", nil), actionSettingsBack),
				).
				Value(&action))
		},
		func() error {
			switch action {
			case thinkingMode:
				return editThinkingMode(current, translator)
			case thinkingShow:
				return editThinkingShow(current, translator)
			case actionSettingsBack, "":
				return errSubmenuBack
			default:
				return nil
			}
		},
	)
}

func editDisplay(current *state, translator *catalog.Catalog) error {
	action := displayLanguage
	return runSubmenu(
		func() error {
			return runForm(context.Background(), translator, huh.NewSelect[string]().
				Title(translator.Text("menu.config.display_language", nil)).
				Options(
					huh.NewOption(
						translator.Text("menu.display.language", nil)+": "+current.config.UI.Language,
						displayLanguage,
					),
					huh.NewOption(
						translator.Text("menu.display.render_mode", nil)+": "+current.config.UI.RenderMode,
						displayRenderMode,
					),
					huh.NewOption(
						translator.Text("menu.display.markdown", nil)+": "+current.config.UI.Markdown,
						displayMarkdown,
					),
					huh.NewOption(
						translator.Text("menu.display.color", nil)+": "+current.config.UI.Color,
						displayColor,
					),
					huh.NewOption(
						translator.Text("menu.display.hyperlinks", nil)+": "+current.config.UI.Hyperlinks,
						displayHyperlinks,
					),
					huh.NewOption(translator.Text("menu.action.back", nil), actionSettingsBack),
				).
				Value(&action))
		},
		func() error {
			switch action {
			case displayLanguage:
				return editDisplayLanguage(current, translator)
			case displayRenderMode:
				return editDisplayRenderMode(current, translator)
			case displayMarkdown:
				return editDisplayMarkdown(current, translator)
			case displayColor:
				return editDisplayColor(current, translator)
			case displayHyperlinks:
				return editDisplayHyperlinks(current, translator)
			case actionSettingsBack, "":
				return errSubmenuBack
			default:
				return nil
			}
		},
	)
}

func runSubmenu(show, edit func() error) error {
	for {
		if err := show(); err != nil {
			if errors.Is(err, errCancelled) {
				return nil
			}
			return err
		}
		if err := edit(); err != nil {
			switch {
			case errors.Is(err, errSubmenuBack):
				return nil
			case errors.Is(err, errCancelled):
				continue
			default:
				return err
			}
		}
	}
}

func editContextMessages(current *state, translator *catalog.Catalog) error {
	value := fmt.Sprintf("%d", current.config.Chat.MaxContextMessages)
	if err := runForm(context.Background(), translator, huh.NewInput().
		Title(translator.Text("menu.context.max_messages", nil)).
		Validate(func(input string) error {
			_, err := parsePositiveInt(input, translator)
			return err
		}).
		Value(&value)); err != nil {
		return err
	}
	parsed, err := parsePositiveInt(value, translator)
	if err != nil {
		return err
	}
	current.config.Chat.MaxContextMessages = parsed
	return nil
}

func editContextTokens(current *state, translator *catalog.Catalog) error {
	value := fmt.Sprintf("%d", current.config.Chat.MaxContextTokens)
	if err := runForm(context.Background(), translator, huh.NewInput().
		Title(translator.Text("menu.context.max_tokens", nil)).
		Validate(func(input string) error {
			_, err := parsePositiveInt(input, translator)
			return err
		}).
		Value(&value)); err != nil {
		return err
	}
	parsed, err := parsePositiveInt(value, translator)
	if err != nil {
		return err
	}
	current.config.Chat.MaxContextTokens = parsed
	return nil
}

func editContextStream(current *state, translator *catalog.Catalog) error {
	value := current.config.Chat.Stream
	if err := runForm(context.Background(), translator, huh.NewConfirm().
		Title(translator.Text("menu.context.stream", nil)).
		Affirmative(translator.Text("menu.action.yes", nil)).
		Negative(translator.Text("menu.action.no", nil)).
		Value(&value)); err != nil {
		return err
	}
	current.config.Chat.Stream = value
	return nil
}

func editContextRetention(current *state, translator *catalog.Catalog) error {
	value := current.config.History.Retention
	if err := editStringSetting(
		translator.Text("menu.context.retention", nil),
		&value,
		retentionOptions(translator),
		translator,
	); err != nil {
		return err
	}
	current.config.History.Retention = value
	return nil
}

func editThinkingMode(current *state, translator *catalog.Catalog) error {
	value := current.config.Reasoning.Mode
	if err := editStringSetting(
		translator.Text("menu.thinking.mode", nil),
		&value,
		modeOptions(),
		translator,
	); err != nil {
		return err
	}
	current.config.Reasoning.Mode = value
	return nil
}

func editThinkingShow(current *state, translator *catalog.Catalog) error {
	value := current.config.Reasoning.Show
	if err := editStringSetting(
		translator.Text("menu.thinking.show", nil),
		&value,
		modeOptions(),
		translator,
	); err != nil {
		return err
	}
	current.config.Reasoning.Show = value
	return nil
}

func editDisplayLanguage(current *state, translator *catalog.Catalog) error {
	value := current.config.UI.Language
	if err := editStringSetting(
		translator.Text("menu.display.language", nil),
		&value,
		[]huh.Option[string]{
			huh.NewOption("auto", "auto"),
			huh.NewOption("zh-CN", "zh-CN"),
			huh.NewOption("en-US", "en-US"),
		},
		translator,
	); err != nil {
		return err
	}
	current.config.UI.Language = value
	return nil
}

func editDisplayRenderMode(current *state, translator *catalog.Catalog) error {
	value := current.config.UI.RenderMode
	if err := editStringSetting(
		translator.Text("menu.display.render_mode", nil),
		&value,
		[]huh.Option[string]{
			huh.NewOption("auto", "auto"),
			huh.NewOption("full", "full"),
			huh.NewOption("inline", "inline"),
			huh.NewOption("plain", "plain"),
		},
		translator,
	); err != nil {
		return err
	}
	current.config.UI.RenderMode = value
	return nil
}

func editDisplayMarkdown(current *state, translator *catalog.Catalog) error {
	value := current.config.UI.Markdown
	if err := editStringSetting(
		translator.Text("menu.display.markdown", nil),
		&value,
		modeOptions(),
		translator,
	); err != nil {
		return err
	}
	current.config.UI.Markdown = value
	return nil
}

func editDisplayColor(current *state, translator *catalog.Catalog) error {
	value := current.config.UI.Color
	if err := editStringSetting(
		translator.Text("menu.display.color", nil),
		&value,
		modeOptions(),
		translator,
	); err != nil {
		return err
	}
	current.config.UI.Color = value
	return nil
}

func editDisplayHyperlinks(current *state, translator *catalog.Catalog) error {
	value := current.config.UI.Hyperlinks
	if err := editStringSetting(
		translator.Text("menu.display.hyperlinks", nil),
		&value,
		modeOptions(),
		translator,
	); err != nil {
		return err
	}
	current.config.UI.Hyperlinks = value
	return nil
}

func editStringSetting(
	title string,
	value *string,
	options []huh.Option[string],
	translator *catalog.Catalog,
) error {
	selected := *value
	if err := runForm(context.Background(), translator, huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(&selected)); err != nil {
		return err
	}
	*value = selected
	return nil
}

func retentionOptions(translator *catalog.Catalog) []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption(translator.Text("menu.context.retention.never", nil), settings.RetentionNever),
		huh.NewOption(translator.Text("menu.context.retention.7d", nil), settings.Retention7Days),
		huh.NewOption(translator.Text("menu.context.retention.30d", nil), settings.Retention30Days),
		huh.NewOption(translator.Text("menu.context.retention.60d", nil), settings.Retention60Days),
	}
}

func modeOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("auto", "auto"),
		huh.NewOption("on", "on"),
		huh.NewOption("off", "off"),
	}
}

func yesNo(value bool, translator *catalog.Catalog) string {
	if value {
		return translator.Text("menu.action.yes", nil)
	}
	return translator.Text("menu.action.no", nil)
}

func saveChanges(
	ctx context.Context,
	service *configure.Service,
	current *state,
	translator *catalog.Catalog,
) error {
	memorySecrets := current.secrets
	testConnection := current.providerDirty()
	loadingMessage := translator.Text("menu.loading.save_config", nil)
	if testConnection {
		loadingMessage = translator.Text("menu.loading.save_and_test", nil)
	}
	return runLoading(
		ctx,
		loadingMessage+" · "+translator.Text("menu.loading.cancel", nil),
		func(workContext context.Context) error {
			previews, err := service.Save(workContext, current.config, current.secrets)
			if err != nil {
				return err
			}
			if len(previews) > 0 {
				current.secrets = map[string]configure.SecretChange{}
			}
			for id, profile := range current.removals {
				temp := cloneConfig(current.config)
				temp.Providers[id] = profile
				_, _, err := service.RemoveSecret(workContext, temp, id)
				if err != nil {
					return err
				}
			}
			current.removals = map[string]settings.Provider{}
			if !testConnection {
				return nil
			}
			providerID := current.config.ActiveProvider
			if providerID == "" {
				return nil
			}
			memorySecret := ""
			if change, ok := memorySecrets[providerID]; ok {
				memorySecret = change.Value
			}
			models, err := service.DiscoverModels(workContext, current.config, providerID, memorySecret)
			if err != nil {
				return errors.New(translator.Text("menu.config.connection_test_failed", map[string]string{
					"error": userError(translator, err),
				}))
			}
			_ = models
			return nil
		},
	)
}

func providerSelectionOptions(
	config settings.Config,
	translator *catalog.Catalog,
	width int,
) []huh.Option[string] {
	ids := sortedProviderIDs(config.Providers)
	options := make([]huh.Option[string], 0, len(ids)+2)
	for _, id := range ids {
		options = append(options, huh.NewOption(formatProvider(id, config.Providers[id], width), id))
	}
	options = append(
		options,
		huh.NewOption(translator.Text("menu.action.add_provider", nil), providerActionAdd),
		huh.NewOption(translator.Text("menu.action.back", nil), providerActionBack),
	)
	return options
}

func providerColumns(translator *catalog.Catalog, width int) string {
	labels := []string{
		translator.Text("menu.provider.columns.provider", nil),
		translator.Text("menu.provider.columns.api_format", nil),
		translator.Text("menu.provider.columns.default_model", nil),
	}
	if width < providerCompactWidth {
		return "  " + widgets.CompactHeader(labels, max(width-2, 1))
	}
	return "  " + widgets.FormatColumns(
		labels,
		providerColumnWidths,
	)
}

func formatProvider(id string, profile settings.Provider, width int) string {
	model := profile.DefaultModel
	if strings.TrimSpace(model) == "" {
		model = "-"
	}
	cells := []string{id, providerFormatLabel(profile.Format), model}
	if width < providerCompactWidth {
		return widgets.FormatCompact(cells, max(width-2, 1))
	}
	return widgets.FormatColumns(
		cells,
		providerColumnWidths,
	)
}

func providerFormatLabel(format string) string {
	switch format {
	case string(provider.FormatOpenAIChatCompletions):
		return "OpenAI Chat"
	case string(provider.FormatOpenAIResponses):
		return "OpenAI Responses"
	case string(provider.FormatAnthropicMessages):
		return "Anthropic Messages"
	default:
		return format
	}
}

func chooseProvider(
	config settings.Config,
	translator *catalog.Catalog,
	title string,
) (string, error) {
	return chooseProviderValue(config, translator, title, "")
}

func chooseProviderValue(
	config settings.Config,
	translator *catalog.Catalog,
	title string,
	initial string,
) (string, error) {
	value := initial
	options := providerIDOptions(config)
	if len(options) == 0 {
		return "", nil
	}
	if err := runForm(context.Background(), translator, huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(&value)); err != nil {
		return "", err
	}
	return value, nil
}

func providerIDOptions(config settings.Config) []huh.Option[string] {
	ids := sortedProviderIDs(config.Providers)
	options := make([]huh.Option[string], 0, len(ids))
	for _, id := range ids {
		options = append(options, huh.NewOption(id, id))
	}
	return options
}

func chooseProviderFrom(
	ids []string,
	translator *catalog.Catalog,
	title string,
) (string, error) {
	value := ""
	options := make([]huh.Option[string], 0, len(ids))
	for _, id := range ids {
		options = append(options, huh.NewOption(id, id))
	}
	if err := runForm(context.Background(), translator, huh.NewSelect[string]().Title(title).Options(options...).Value(&value)); err != nil {
		return "", err
	}
	return value, nil
}

func cloneConfig(input settings.Config) settings.Config {
	output := input
	output.Providers = make(map[string]settings.Provider, len(input.Providers))
	for id, profile := range input.Providers {
		profile.EnabledModels = append([]string(nil), profile.EnabledModels...)
		profile.Headers = cloneMap(profile.Headers)
		output.Providers[id] = profile
	}
	return output
}

func cloneMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func providerIDs(providers map[string]settings.Provider, exclude string) []string {
	result := make([]string, 0, len(providers))
	for id := range providers {
		if id != exclude {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func sortedProviderIDs(providers map[string]settings.Provider) []string {
	return providerIDs(providers, "")
}

func initialProviderSelection(config settings.Config) string {
	ids := sortedProviderIDs(config.Providers)
	if len(ids) > 0 {
		return ids[0]
	}
	return providerActionAdd
}

func validateProviderID(value string, translator *catalog.Catalog) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New(translator.Text("menu.provider.validation.required", nil))
	}
	for index, char := range value {
		if index == 0 && (char < 'a' || char > 'z') {
			return errors.New(translator.Text("menu.provider.validation.lowercase_start", nil))
		}
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return errors.New(translator.Text("menu.provider.validation.characters", nil))
		}
	}
	return nil
}

func validateBaseURL(value string, translator *catalog.Catalog) error {
	_, err := validation.NormalizeBaseURL(value)
	if err == nil {
		return nil
	}
	return errors.New(userError(translator, err))
}

func parsePositiveInt(value string, translator *catalog.Catalog) (int, error) {
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return 0, errors.New(translator.Text("menu.validation.positive_integer", nil))
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func rootMenuOptions(translator *catalog.Catalog, providerDirty bool) []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption(translator.Text("menu.config.provider_management", nil), actionProviders),
		huh.NewOption(translator.Text("menu.config.default_model", nil), actionDefaultModel),
		huh.NewOption(translator.Text("menu.config.system_prompt", nil), actionSystem),
		huh.NewOption(translator.Text("menu.config.conversation_context", nil), actionContext),
		huh.NewOption(translator.Text("menu.config.thinking_mode", nil), actionThinking),
		huh.NewOption(translator.Text("menu.config.display_language", nil), actionDisplay),
		huh.NewOption(saveActionLabel(providerDirty, translator), actionSave),
	}
}

func saveActionLabel(providerDirty bool, translator *catalog.Catalog) string {
	if providerDirty {
		return translator.Text("menu.action.save_and_test", nil)
	}
	return translator.Text("menu.action.save", nil)
}

func (s *state) dirty() bool {
	return !reflect.DeepEqual(s.config, s.initial) ||
		len(s.secrets) > 0 ||
		len(s.removals) > 0
}

func (s *state) providerDirty() bool {
	return !reflect.DeepEqual(s.config.ActiveProvider, s.initial.ActiveProvider) ||
		!reflect.DeepEqual(s.config.Providers, s.initial.Providers) ||
		!reflect.DeepEqual(s.config.Model, s.initial.Model) ||
		len(s.secrets) > 0 ||
		len(s.removals) > 0
}

func menuDescription(config settings.Config, translator *catalog.Catalog) string {
	return translator.Text("menu.config.summary", map[string]string{
		"providers": fmt.Sprintf("%d", len(config.Providers)),
		"active":    emptyAsDash(config.ActiveProvider),
		"retention": retentionLabel(config.History.Retention, translator),
	})
}

func retentionLabel(value string, translator *catalog.Catalog) string {
	switch value {
	case settings.Retention7Days, settings.Retention30Days, settings.Retention60Days, settings.RetentionNever:
		return translator.Text("menu.context.retention."+value, nil)
	default:
		return emptyAsDash(value)
	}
}

func userError(translator *catalog.Catalog, err error) string {
	var failureValue *failure.Error
	if errors.As(err, &failureValue) {
		return translator.Error(err)
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
