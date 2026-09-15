package config

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/KevinLeeNJ/ask/internal/application/configure"
	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
)

func TestRootMenuMovesProviderSettingsIntoProviderManagement(t *testing.T) {
	options := rootMenuOptions(catalog.New("zh-CN"), false)
	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, option.Value)
	}
	want := []string{
		actionProviders,
		actionDefaultModel,
		actionSystem,
		actionContext,
		actionThinking,
		actionDisplay,
		actionSave,
	}
	if len(values) != len(want) {
		t.Fatalf("menu values = %#v, want %#v", values, want)
	}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("menu values = %#v, want %#v", values, want)
		}
	}
	if options[1].Key != "默认模型" {
		t.Fatalf("default model menu label = %q", options[1].Key)
	}
	if options[6].Key != "保存" {
		t.Fatalf("save menu label = %q", options[6].Key)
	}
}

func TestStateDirtyTracksConfigAndPendingWrites(t *testing.T) {
	initial := settings.Default()
	current := &state{
		initial:  cloneConfig(initial),
		config:   cloneConfig(initial),
		secrets:  map[string]configure.SecretChange{},
		removals: map[string]settings.Provider{},
	}
	if current.dirty() {
		t.Fatal("unchanged state reported dirty")
	}

	current.config.Chat.SystemPrompt = "changed"
	if !current.dirty() {
		t.Fatal("changed config did not report dirty")
	}

	current.config = cloneConfig(initial)
	current.secrets["work"] = configure.SecretChange{Value: "secret"}
	if !current.dirty() {
		t.Fatal("pending secret did not report dirty")
	}
}

func TestStateProviderDirtyOnlyTracksProviderAndModelChanges(t *testing.T) {
	initial := settings.Default()
	current := &state{
		initial:  cloneConfig(initial),
		config:   cloneConfig(initial),
		secrets:  map[string]configure.SecretChange{},
		removals: map[string]settings.Provider{},
	}

	current.config.Chat.SystemPrompt = "changed"
	current.config.UI.Color = "off"
	current.config.History.Retention = settings.Retention7Days
	if current.providerDirty() {
		t.Fatal("non-provider changes marked provider configuration dirty")
	}

	current.config.Providers["work"] = settings.Provider{DefaultModel: "model-a"}
	if !current.providerDirty() {
		t.Fatal("provider change was not marked provider configuration dirty")
	}

	current.config = cloneConfig(initial)
	current.secrets["work"] = configure.SecretChange{Value: "secret"}
	if !current.providerDirty() {
		t.Fatal("pending secret was not marked provider configuration dirty")
	}
}

func TestSaveActionLabelChangesOnlyForProviderChanges(t *testing.T) {
	translator := catalog.New("zh-CN")
	if got := saveActionLabel(false, translator); got != "保存" {
		t.Fatalf("save label = %q", got)
	}
	if got := saveActionLabel(true, translator); got != "保存并测试" {
		t.Fatalf("save and test label = %q", got)
	}
}

func TestResolveEditedValueSkipsConfirmationWhenUnchanged(t *testing.T) {
	confirmed := false
	edited, save, err := resolveEditedValue("same", "same", func() (bool, error) {
		confirmed = true
		return true, nil
	})
	if err != nil {
		t.Fatalf("resolveEditedValue() error = %v", err)
	}
	if confirmed || save || edited != "same" {
		t.Fatalf("result = edited:%q save:%v confirmed:%v", edited, save, confirmed)
	}
}

func TestResolveEditedValueConfirmsChangedValue(t *testing.T) {
	edited, save, err := resolveEditedValue("old", "new", func() (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("resolveEditedValue() error = %v", err)
	}
	if edited != "new" || !save {
		t.Fatalf("result = edited:%q save:%v", edited, save)
	}
}

func TestResolveEditedValuePreservesEditedValueWhenDiscarded(t *testing.T) {
	edited, save, err := resolveEditedValue("old", "new", func() (bool, error) {
		return false, nil
	})
	if err != nil {
		t.Fatalf("resolveEditedValue() error = %v", err)
	}
	if edited != "new" || save {
		t.Fatalf("result = edited:%q save:%v", edited, save)
	}
}

func TestProviderProfileParsesConnectionSettings(t *testing.T) {
	profile, err := providerProfile(
		"openai-responses",
		"https://example.com/",
		"45s",
		catalog.New("zh-CN"),
	)
	if err != nil {
		t.Fatalf("providerProfile() error = %v", err)
	}
	if profile.Format != "openai-responses" {
		t.Fatalf("format = %q", profile.Format)
	}
	if profile.BaseURL != "https://example.com" {
		t.Fatalf("base URL = %q", profile.BaseURL)
	}
	if profile.RequestTimeout != 45*time.Second {
		t.Fatalf("timeout = %s", profile.RequestTimeout)
	}
}

func TestProviderSelectionListsProvidersBeforeActions(t *testing.T) {
	config := settings.Default()
	config.Providers["work"] = settings.Provider{
		Format:       string(provider.FormatOpenAIResponses),
		DefaultModel: "gpt-5",
	}
	config.Providers["local"] = settings.Provider{
		Format:       string(provider.FormatAnthropicMessages),
		DefaultModel: "claude-sonnet",
	}

	options := providerSelectionOptions(config, catalog.New("zh-CN"), 80)
	if len(options) != 4 {
		t.Fatalf("option count = %d, want 4", len(options))
	}
	if options[0].Value != "local" || options[1].Value != "work" {
		t.Fatalf("provider options = %#v", options[:2])
	}
	if options[2].Value != providerActionAdd || options[3].Value != providerActionBack {
		t.Fatalf("action options = %#v", options[2:])
	}
}

func TestInitialProviderSelectionRestoresEntry(t *testing.T) {
	config := settings.Default()
	if got := initialProviderSelection(config); got != providerActionAdd {
		t.Fatalf("empty provider selection = %q, want add", got)
	}
	config.Providers["work"] = settings.Provider{}
	config.Providers["local"] = settings.Provider{}
	if got := initialProviderSelection(config); got != "local" {
		t.Fatalf("provider selection = %q, want local", got)
	}
}

func TestProviderIDOptionsRemainPlainForDefaultModel(t *testing.T) {
	config := settings.Default()
	config.Providers["work"] = settings.Provider{
		Format:       string(provider.FormatOpenAIResponses),
		DefaultModel: "gpt-5",
	}

	options := providerIDOptions(config)
	if len(options) != 1 {
		t.Fatalf("option count = %d, want 1", len(options))
	}
	if options[0].Key != "work" || options[0].Value != "work" {
		t.Fatalf("provider ID option = %#v, want plain work option", options[0])
	}
}

func TestProviderColumnsAlignWithRows(t *testing.T) {
	translator := catalog.New("zh-CN")
	header := providerColumns(translator, 80)
	row := formatProvider("work", settings.Provider{
		Format:       string(provider.FormatOpenAIResponses),
		DefaultModel: "gpt-5",
	}, 80)

	if got, want := providerColumnOffset("  "+row, "OpenAI Responses"), providerColumnOffset(header, "接口格式"); got != want {
		t.Fatalf("format column offset = %d, want %d", got, want)
	}
	if got, want := providerColumnOffset("  "+row, "gpt-5"), providerColumnOffset(header, "默认模型"); got != want {
		t.Fatalf("model column offset = %d, want %d", got, want)
	}
}

func TestCompactProviderColumnsFitTerminal(t *testing.T) {
	translator := catalog.New("zh-CN")
	header := providerColumns(translator, 50)
	row := formatProvider("work", settings.Provider{
		Format:       string(provider.FormatOpenAIResponses),
		DefaultModel: "gpt-5",
	}, 50)
	for name, value := range map[string]string{"header": header, "row": row} {
		if got := ansi.StringWidth(value); got > 50 {
			t.Fatalf("%s width = %d, want <= 50: %q", name, got, value)
		}
	}
}

func TestRetentionLabelIsLocalized(t *testing.T) {
	translator := catalog.New("zh-CN")
	if got, want := retentionLabel(settings.RetentionNever, translator), "不自动删除"; got != want {
		t.Fatalf("retentionLabel() = %q, want %q", got, want)
	}
	if got := menuDescription(settings.Default(), translator); !strings.Contains(got, "保留：不自动删除") {
		t.Fatalf("menuDescription() = %q", got)
	}
}

func TestUserErrorLocalizesStableFailures(t *testing.T) {
	translator := catalog.New("en-US")
	err := failure.New(failure.KindConfig, "config.base_url_empty", nil)
	if got, want := userError(translator, err), "Base URL is required"; got != want {
		t.Fatalf("userError() = %q, want %q", got, want)
	}
}

func TestSaveChangesSkipsConnectionTestForNonProviderSettings(t *testing.T) {
	t.Setenv("TERM", "dumb")
	initial := settings.Default()
	initial.ActiveProvider = "work"
	initial.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Second,
	}
	current := &state{
		initial:  cloneConfig(initial),
		config:   cloneConfig(initial),
		secrets:  map[string]configure.SecretChange{},
		removals: map[string]settings.Provider{},
	}
	current.config.Chat.SystemPrompt = "changed"

	store := &savingConfigStore{}
	secrets := &countingSecrets{}
	resolver := &countingResolver{}
	service := &configure.Service{
		Configs:  store,
		Secrets:  secrets,
		Registry: resolver,
		Shell:    testShellProfileWriter{},
	}

	if err := saveChanges(context.Background(), service, current, catalog.New("en-US")); err != nil {
		t.Fatalf("saveChanges() error = %v", err)
	}
	if !store.saved {
		t.Fatal("configuration was not saved")
	}
	if secrets.calls != 0 || resolver.calls != 0 {
		t.Fatalf("connection test ran: secrets=%d resolver=%d", secrets.calls, resolver.calls)
	}
}

func TestSaveChangesTestsConnectionForProviderSettings(t *testing.T) {
	t.Setenv("TERM", "dumb")
	initial := settings.Default()
	initial.ActiveProvider = "work"
	initial.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Second,
	}
	current := &state{
		initial:  cloneConfig(initial),
		config:   cloneConfig(initial),
		secrets:  map[string]configure.SecretChange{},
		removals: map[string]settings.Provider{},
	}
	updated := current.config.Providers["work"]
	updated.DefaultModel = "model-b"
	updated.EnabledModels = []string{"model-a", "model-b"}
	current.config.Providers["work"] = updated

	store := &savingConfigStore{}
	secrets := &countingSecrets{}
	resolver := &countingResolver{}
	service := &configure.Service{
		Configs:  store,
		Secrets:  secrets,
		Registry: resolver,
		Shell:    testShellProfileWriter{},
	}

	if err := saveChanges(context.Background(), service, current, catalog.New("en-US")); err == nil {
		t.Fatal("saveChanges() unexpectedly skipped provider connection test")
	}
	if secrets.calls == 0 || resolver.calls == 0 {
		t.Fatalf("connection test did not run: secrets=%d resolver=%d", secrets.calls, resolver.calls)
	}
}

func providerColumnOffset(line, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	return ansi.StringWidth(line[:index])
}

type savingConfigStore struct {
	saved bool
}

func (s *savingConfigStore) Load(context.Context) (settings.Config, error) {
	return settings.Config{}, nil
}

func (s *savingConfigStore) Save(context.Context, settings.Config) error {
	s.saved = true
	return nil
}

type countingSecrets struct {
	calls int
}

func (s *countingSecrets) Resolve(context.Context, provider.ID) (ports.Secret, error) {
	s.calls++
	return ports.Secret{}, nil
}

type countingResolver struct {
	calls int
}

func (r *countingResolver) Resolve(settings.Provider, ports.Secret) (ports.ResolvedProvider, error) {
	r.calls++
	return ports.ResolvedProvider{}, nil
}

type testShellProfileWriter struct{}

func (testShellProfileWriter) Defaults() (ports.ShellProfileDefaults, error) {
	return ports.ShellProfileDefaults{
		Shell:       "zsh",
		Syntax:      "posix",
		ProfileFile: "~/.zshrc",
	}, nil
}

func (testShellProfileWriter) Preview(ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	return ports.ShellProfilePreview{}, nil
}

func (testShellProfileWriter) Write(ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	return ports.ShellProfilePreview{}, nil
}

func (testShellProfileWriter) Remove(ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	return ports.ShellProfilePreview{}, nil
}
