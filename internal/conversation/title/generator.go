package title

import (
	"context"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

type Generator struct {
	Client    ports.ChatProvider
	Route     provider.Route
	SessionID string
}

func (g Generator) Generate(ctx context.Context, question string) (string, error) {
	if g.Client == nil {
		return "", failure.New(failure.KindInternal, "title.client_missing", nil)
	}
	response, err := g.Client.Complete(ctx, provider.Request{
		Route:        g.Route,
		SessionID:    g.SessionID,
		SystemPrompt: SystemPrompt(),
		Messages: []provider.Message{{
			Role:    provider.RoleUser,
			Content: RequestText(question),
		}},
		Stream:          false,
		MaxOutputTokens: 64,
		Thinking: provider.ThinkingRequest{
			Explicit:    true,
			Enabled:     false,
			OmitWhenOff: true,
		},
	})
	if err != nil {
		return "", err
	}
	normalized := NormalizeGenerated(response.Content)
	if normalized == "" {
		return "", failure.New(failure.KindProtocol, "title.empty", nil)
	}
	return normalized, nil
}
