package ask

import (
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestResolveRoutePrefersConversationRouteOverActiveProvider(t *testing.T) {
	config := routeTestConfig()

	route, profile, err := resolveRoute(
		config,
		"",
		"",
		provider.Route{Provider: "work-alt", Model: "model-alt"},
	)
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.Provider != "work-alt" || route.Model != "model-alt" {
		t.Fatalf("route = %#v", route)
	}
	if profile.BaseURL != "https://alt.example.com" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestResolveRouteKeepsExplicitOverridesAboveConversationRoute(t *testing.T) {
	config := routeTestConfig()
	conversationRoute := provider.Route{Provider: "work-alt", Model: "model-alt"}

	route, _, err := resolveRoute(config, "", "model-b", conversationRoute)
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.Provider != "work-alt" || route.Model != "model-b" {
		t.Fatalf("model override route = %#v", route)
	}

	route, _, err = resolveRoute(config, "work", "", conversationRoute)
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.Provider != "work" || route.Model != "model-a" {
		t.Fatalf("provider override route = %#v", route)
	}
}

func TestResolveRouteFallsBackWhenConversationProviderWasRemoved(t *testing.T) {
	config := routeTestConfig()

	route, _, err := resolveRoute(
		config,
		"",
		"",
		provider.Route{Provider: "removed", Model: "removed-model"},
	)
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.Provider != "work" || route.Model != "model-a" {
		t.Fatalf("route = %#v", route)
	}
}

func routeTestConfig() settings.Config {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://work.example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a", "model-b"},
		RequestTimeout: 5 * time.Second,
	}
	config.Providers["work-alt"] = settings.Provider{
		Format:         string(provider.FormatOpenAIResponses),
		BaseURL:        "https://alt.example.com",
		DefaultModel:   "model-alt-default",
		EnabledModels:  []string{"model-alt", "model-alt-default"},
		RequestTimeout: 5 * time.Second,
	}
	return config
}
