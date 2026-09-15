package validation

import (
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := map[string]string{
		"https://api.openai.com":                                "https://api.openai.com",
		"https://api.openai.com/":                               "https://api.openai.com",
		"https://api.openai.com/v1":                             "https://api.openai.com",
		"https://api.openai.com/v1/chat/completions":            "https://api.openai.com",
		"https://api.openai.com/v1/responses":                   "https://api.openai.com",
		"https://api.anthropic.com/v1/messages":                 "https://api.anthropic.com",
		"https://gateway.example.com/openai-proxy/v1":           "https://gateway.example.com/openai-proxy",
		"https://gateway.example.com/openai-proxy/v1/responses": "https://gateway.example.com/openai-proxy",
	}
	for input, want := range tests {
		got, err := NormalizeBaseURL(input)
		if err != nil {
			t.Errorf("NormalizeBaseURL(%q) error = %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeBaseURLRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "ftp://example.com", "not-a-url", "https://example.com?a=1", "https://example.com#x"} {
		if _, err := NormalizeBaseURL(input); err == nil {
			t.Errorf("NormalizeBaseURL(%q) unexpectedly succeeded", input)
		}
	}
}

func TestNormalizeBaseURLReturnsStableFailure(t *testing.T) {
	_, err := NormalizeBaseURL("ftp://example.com")
	if failure.KindOf(err) != failure.KindConfig {
		t.Fatalf("NormalizeBaseURL() error = %v, kind = %s", err, failure.KindOf(err))
	}
	if info := failure.Info(err); info.MessageID != "config.base_url_scheme_invalid" {
		t.Fatalf("error info = %#v", info)
	}
}

func TestValidateProviderRules(t *testing.T) {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers = map[string]settings.Provider{
		"work": {
			Format:         string(provider.FormatOpenAIResponses),
			BaseURL:        "https://example.com",
			DefaultModel:   "model-a",
			EnabledModels:  []string{"model-a"},
			RequestTimeout: 30 * time.Second,
		},
	}
	if err := Validate(config); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidFormat(t *testing.T) {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers = map[string]settings.Provider{
		"work": {
			Format:         "unsupported",
			BaseURL:        "https://example.com",
			DefaultModel:   "model-a",
			EnabledModels:  []string{"model-a"},
			RequestTimeout: 30 * time.Second,
		},
	}
	err := Validate(config)
	if failure.KindOf(err) != failure.KindConfig {
		t.Fatalf("Validate() error = %v, kind = %s", err, failure.KindOf(err))
	}
	info := failure.Info(err)
	if info.MessageID != "config.provider_format_invalid" {
		t.Fatalf("error info = %#v", info)
	}
}
