package capability

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestResolverReadsMinimumReasoningEffortFromModelsDev(t *testing.T) {
	client := &staticHTTPDoer{
		statusCode: http.StatusOK,
		body: `{
			"opencode-go": {
				"models": {
					"deepseek-v4.1-flash": {
						"reasoning": true,
						"reasoning_options": [
							{"type":"effort","values":["low","high","max"]}
						]
					}
				}
			}
		}`,
	}
	cache := &memoryCapabilityCache{}
	resolver := New(client, cache)

	value, ok, err := resolver.MinimumReasoningEffort(
		context.Background(),
		provider.Route{Provider: "opencode", Model: "deepseek-v4.1-flash"},
		settings.Provider{BaseURL: "https://opencode.ai/zen/go"},
	)
	if err != nil {
		t.Fatalf("MinimumReasoningEffort() error = %v", err)
	}
	if !ok || value != "low" {
		t.Fatalf("MinimumReasoningEffort() = %q, %v", value, ok)
	}
	if client.calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", client.calls)
	}

	value, ok, err = resolver.MinimumReasoningEffort(
		context.Background(),
		provider.Route{Provider: "opencode", Model: "deepseek-v4.1-flash"},
		settings.Provider{BaseURL: "https://opencode.ai/zen/go"},
	)
	if err != nil || !ok || value != "low" {
		t.Fatalf("cached MinimumReasoningEffort() = %q, %v, %v", value, ok, err)
	}
	if client.calls != 1 {
		t.Fatalf("HTTP calls after cache = %d, want 1", client.calls)
	}
}

func TestMinimumEffortPrefersLowestKnownRank(t *testing.T) {
	if got := minimumEffort([]string{"max", "medium", "low"}); got != "low" {
		t.Fatalf("minimumEffort() = %q, want low", got)
	}
	if got := minimumEffort([]string{"custom-b", "custom-a"}); got != "custom-b" {
		t.Fatalf("minimumEffort() = %q, want first unknown value", got)
	}
}

func TestModelMatchAcceptsProviderQualifiedNames(t *testing.T) {
	if rank := modelMatchRank("myprovider/deepseek-v4.1-flash", "deepseek/deepseek-v4.1-flash"); rank != 1 {
		t.Fatalf("modelMatchRank() = %d, want 1", rank)
	}
	if rank := modelMatchRank("deepseek-v4.1-flash", "deepseek/deepseek-v4.1-flash"); rank != 1 {
		t.Fatalf("modelMatchRank() = %d, want 1", rank)
	}
	if rank := modelMatchRank("deepseek-v4.1-flash", "deepseek-v4.1-flash"); rank != 0 {
		t.Fatalf("modelMatchRank() = %d, want 0", rank)
	}
}

func TestPreferredProviderWinsOverGlobalSuffixMatch(t *testing.T) {
	catalog := modelsDevCatalog{
		"opencode-go": {
			Models: map[string]modelsDevModel{
				"deepseek-v4.1-flash": effortModel("low"),
			},
		},
		"other": {
			Models: map[string]modelsDevModel{
				"deepseek/deepseek-v4.1-flash": effortModel("high"),
			},
		},
	}
	matches := matchingReasoningCapabilities(catalog, "deepseek-v4.1-flash")
	if value, ok := preferredMatchValue(matches, "opencode-go"); !ok || value != "low" {
		t.Fatalf("preferredMatchValue() = %q, %v", value, ok)
	}
}

func TestGlobalSuffixMatchRejectsConflictingValues(t *testing.T) {
	catalog := modelsDevCatalog{
		"provider-a": {
			Models: map[string]modelsDevModel{
				"vendor/deepseek-v4.1-flash": effortModel("low"),
			},
		},
		"provider-b": {
			Models: map[string]modelsDevModel{
				"other/deepseek-v4.1-flash": effortModel("high"),
			},
		},
	}
	matches := matchingReasoningCapabilities(catalog, "myprovider/deepseek-v4.1-flash")
	if value, ok := unambiguousMatchValue(matches); ok {
		t.Fatalf("unambiguousMatchValue() = %q, want no match", value)
	}
}

func TestModelFamilyNamespaceWinsOverUnqualifiedLookalike(t *testing.T) {
	catalog := modelsDevCatalog{
		"deepseek": {
			Models: map[string]modelsDevModel{
				"deepseek/deepseek-v4.1-flash": effortModel("low"),
			},
		},
		"lookalike": {
			Models: map[string]modelsDevModel{
				"deepseek-v4.1-flash": effortModel("high"),
			},
		},
	}
	matches := matchingReasoningCapabilities(catalog, "myprovider/deepseek-v4.1-flash")
	if value, ok := unambiguousMatchValue(matches); !ok || value != "low" {
		t.Fatalf("unambiguousMatchValue() = %q, %v, want low", value, ok)
	}
}

func effortModel(value string) modelsDevModel {
	model := modelsDevModel{Reasoning: true}
	model.ReasoningOptions = append(model.ReasoningOptions, struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	}{
		Type:   "effort",
		Values: []string{value},
	})
	return model
}

type staticHTTPDoer struct {
	statusCode int
	body       string
	calls      int
}

func (c *staticHTTPDoer) Do(*http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{
		StatusCode: c.statusCode,
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Header:     make(http.Header),
	}, nil
}

type memoryCapabilityCache struct {
	values map[string]string
}

func (c *memoryCapabilityCache) ModelCapability(_ context.Context, key string) (string, bool, error) {
	if c.values == nil {
		return "", false, nil
	}
	value, ok := c.values[key]
	return value, ok, nil
}

func (c *memoryCapabilityCache) SaveModelCapability(_ context.Context, key string, value string) error {
	if c.values == nil {
		c.values = map[string]string{}
	}
	c.values[key] = value
	return nil
}
