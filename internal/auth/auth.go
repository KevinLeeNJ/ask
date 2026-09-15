package auth

import (
	"context"
	"os"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

type EnvironmentResolver struct{}

func (EnvironmentResolver) Resolve(ctx context.Context, providerID provider.ID) (ports.Secret, error) {
	select {
	case <-ctx.Done():
		return ports.Secret{}, ctx.Err()
	default:
	}
	envVar := provider.EnvVar(providerID)
	value, ok := os.LookupEnv(envVar)
	if !ok || strings.TrimSpace(value) == "" {
		return ports.Secret{}, failure.New(
			failure.KindConfig,
			"auth.api_key_missing",
			map[string]string{
				"provider": string(providerID),
				"env_var":  envVar,
			},
		)
	}
	return ports.Secret{
		Value:    value,
		EnvVar:   envVar,
		Provider: providerID,
	}, nil
}

func BuildHeaders(
	format provider.Format,
	secret ports.Secret,
	custom map[string]string,
) (map[string]string, error) {
	headers := make(map[string]string, len(custom)+3)
	for name, value := range custom {
		headers[name] = value
	}
	switch format {
	case provider.FormatOpenAIChatCompletions, provider.FormatOpenAIResponses:
		headers["Authorization"] = "Bearer " + secret.Value
	case provider.FormatAnthropicMessages:
		headers["x-api-key"] = secret.Value
	default:
		return nil, failure.New(
			failure.KindConfig,
			"config.provider_format_invalid",
			map[string]string{"value": string(format)},
		)
	}
	return headers, nil
}
