package registry

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/auth"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	anthropicmessages "github.com/KevinLeeNJ/ask/internal/provider/anthropic/messages"
	"github.com/KevinLeeNJ/ask/internal/provider/openai/chat"
	"github.com/KevinLeeNJ/ask/internal/provider/openai/responses"
)

type Registry struct {
	client ports.HTTPDoer
}

func New(client ports.HTTPDoer) *Registry {
	return &Registry{client: client}
}

func (r *Registry) Resolve(profile settings.Provider, secret ports.Secret) (ports.ResolvedProvider, error) {
	format := provider.Format(profile.Format)
	if !format.Valid() {
		return ports.ResolvedProvider{}, failure.New(
			failure.KindConfig,
			"config.provider_format_invalid",
			map[string]string{"provider": string(secret.Provider), "value": profile.Format},
		)
	}
	headers, err := auth.BuildHeaders(format, secret, profile.Headers)
	if err != nil {
		return ports.ResolvedProvider{}, err
	}
	config := ports.ProviderHTTPConfig{
		ProviderID:      secret.Provider,
		Format:          format,
		BaseURL:         profile.BaseURL,
		Headers:         headers,
		SessionIDHeader: sessionIDHeader(profile.BaseURL),
		Client:          r.client,
	}
	switch format {
	case provider.FormatOpenAIChatCompletions:
		client := chat.New(config)
		return ports.ResolvedProvider{Chat: client, Catalog: client}, nil
	case provider.FormatOpenAIResponses:
		client := responses.New(config)
		return ports.ResolvedProvider{Chat: client, Catalog: client}, nil
	case provider.FormatAnthropicMessages:
		client := anthropicmessages.New(config)
		return ports.ResolvedProvider{Chat: client, Catalog: client}, nil
	default:
		return ports.ResolvedProvider{}, fmt.Errorf("unsupported provider format %q", format)
	}
}

func sessionIDHeader(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "opencode.ai") {
		return ""
	}
	cleanPath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if cleanPath != "/zen/go" && !strings.HasPrefix(cleanPath, "/zen/go/") {
		return ""
	}
	return "x-opencode-session"
}
