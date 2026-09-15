package toml

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

const sampleConfig = `version = 1
active_provider = "openai-main"

[model]
auto_discover = true
temperature = 0.7
max_output_tokens = 4096

[providers.openai-main]
format = "openai_responses"
base_url = "https://api.openai.com/v1/responses"
default_model = "gpt-5-mini"
enabled_models = ["gpt-5-mini", "gpt-5"]
request_timeout = "60s"

[history]
retention = "30d"

[chat]
system_prompt = "system"
max_context_messages = 40
max_context_tokens = 32000
stream = true

[reasoning]
mode = "auto"
strategy = "auto"
request_field = "reasoning_effort"
enabled_value = "minimum"
disabled_value = "none"
disabled_behavior = "send"
show = "auto"
collapse_when_answer_starts = true
anthropic_budget_tokens = "minimum"

[ui]
language = "auto"
render_mode = "auto"
color = "auto"
markdown = "auto"
markdown_theme = "auto"
syntax_highlight = true
hyperlinks = "auto"
show_usage = true
`

func TestDefaultPathUsesHomeConfigDirectory(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("home config directory rule applies to macOS and Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASK_CONFIG_FILE", "")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, ".config", "ask", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathHonorsConfigFileOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom", "config.toml")
	t.Setenv("ASK_CONFIG_FILE", "  "+override+"  ")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if got != override {
		t.Fatalf("DefaultPath() = %q, want %q", got, override)
	}
}

func TestStoreLoadNormalizesAndAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := config.Providers["openai-main"].BaseURL; got != "https://api.openai.com" {
		t.Fatalf("BaseURL = %q", got)
	}
	if config.History.Retention != settings.Retention30Days {
		t.Fatalf("retention = %q", config.History.Retention)
	}
	if !config.UI.SyntaxHighlight {
		t.Fatal("expected UI defaults and explicit values to be preserved")
	}
}

func TestStoreSaveRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         "openai_chat_completions",
		BaseURL:        "https://gateway.example.com/openai-proxy",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: 45 * time.Second,
	}
	store := NewStore(path)
	if err := store.Save(context.Background(), config); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "scheme") {
		t.Fatalf("saved config must not expose auth scheme:\n%s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %o, want 600", got)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Providers["work"].DefaultModel != "model-a" {
		t.Fatalf("loaded config = %#v", loaded.Providers["work"])
	}
}

func TestStoreRejectsSecretFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := strings.Replace(
		sampleConfig,
		`[providers.openai-main]`,
		"[providers.openai-main]\napi_key = \"secret\"",
		1,
	)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewStore(path).Load(context.Background())
	if err == nil {
		t.Fatal("expected secret field error")
	}
}

func TestStoreIgnoresLegacyProviderName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := strings.Replace(
		sampleConfig,
		`[providers.openai-main]`,
		"[providers.openai-main]\nname = \"OpenAI\"",
		1,
	)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("legacy name field should be ignored: %v", err)
	}
	if _, ok := config.Providers["openai-main"]; !ok {
		t.Fatal("provider was not loaded")
	}
}
