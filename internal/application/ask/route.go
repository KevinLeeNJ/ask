package ask

import (
	"strings"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func resolveRoute(
	config settings.Config,
	providerOverride string,
	modelOverride string,
	conversationRoute provider.Route,
) (provider.Route, settings.Provider, error) {
	providerID := strings.TrimSpace(providerOverride)
	model := strings.TrimSpace(modelOverride)
	conversationProvider := strings.TrimSpace(string(conversationRoute.Provider))
	conversationModel := strings.TrimSpace(conversationRoute.Model)
	useConversationRoute := false

	if providerID == "" && conversationProvider != "" {
		if _, ok := config.Providers[conversationProvider]; ok {
			providerID = conversationProvider
			useConversationRoute = true
		}
	}
	if providerID == "" {
		providerID = config.ActiveProvider
	}
	if providerID == "" {
		return provider.Route{}, settings.Provider{}, failure.New(
			failure.KindConfig,
			"config.active_provider_missing",
			nil,
		)
	}
	profile, ok := config.Providers[providerID]
	if !ok {
		return provider.Route{}, settings.Provider{}, failure.New(
			failure.KindConfig,
			"route.provider_unknown",
			map[string]string{"provider": providerID},
		)
	}
	if model == "" {
		if useConversationRoute && conversationModel != "" {
			model = conversationModel
		} else {
			model = profile.DefaultModel
		}
	}
	if strings.TrimSpace(model) == "" {
		return provider.Route{}, settings.Provider{}, failure.New(
			failure.KindConfig,
			"route.model_missing",
			map[string]string{"provider": providerID},
		)
	}
	return provider.Route{Provider: provider.ID(providerID), Model: model}, profile, nil
}
