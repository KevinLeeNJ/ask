package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

const (
	defaultEndpoint = "https://models.dev/api.json"
	cacheTTL        = 24 * time.Hour
)

type modelsDevCatalog map[string]modelsDevProvider

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	Reasoning        bool `json:"reasoning"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
}

type capabilityMatch struct {
	provider string
	model    string
	rank     int
	value    string
}

type Resolver struct {
	Client   ports.HTTPDoer
	Cache    ports.ModelCapabilityCache
	Endpoint string
	Now      func() time.Time
}

func New(client ports.HTTPDoer, cache ports.ModelCapabilityCache) *Resolver {
	return &Resolver{
		Client:   client,
		Cache:    cache,
		Endpoint: defaultEndpoint,
		Now:      time.Now,
	}
}

func (r *Resolver) MinimumReasoningEffort(
	ctx context.Context,
	route provider.Route,
	profile settings.Provider,
) (string, bool, error) {
	if r == nil || r.Client == nil {
		return "", false, nil
	}
	key := capabilityKey(route, profile)
	if value, ok := r.cached(ctx, key); ok {
		return value, true, nil
	}

	lookupContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(lookupContext, http.MethodGet, r.endpoint(), nil)
	if err != nil {
		return "", false, err
	}
	response, err := r.Client.Do(request)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", false, fmt.Errorf("models.dev returned HTTP %d", response.StatusCode)
	}

	var catalog modelsDevCatalog
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32<<20))
	if err := decoder.Decode(&catalog); err != nil {
		return "", false, err
	}

	matches := matchingReasoningCapabilities(catalog, route.Model)
	if len(matches) == 0 {
		return "", false, nil
	}
	for _, candidate := range providerCandidates(route, profile) {
		if value, ok := preferredMatchValue(matches, candidate); ok {
			r.save(ctx, key, value)
			return value, true, nil
		}
	}
	if value, ok := unambiguousMatchValue(matches); ok {
		r.save(ctx, key, value)
		return value, true, nil
	}
	return "", false, nil
}

func (r *Resolver) endpoint() string {
	if strings.TrimSpace(r.Endpoint) != "" {
		return r.Endpoint
	}
	return defaultEndpoint
}

func (r *Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Resolver) cached(ctx context.Context, key string) (string, bool) {
	if r.Cache == nil {
		return "", false
	}
	raw, ok, err := r.Cache.ModelCapability(ctx, key)
	if err != nil || !ok {
		return "", false
	}
	var entry struct {
		Value     string `json:"value"`
		FetchedAt int64  `json:"fetched_at"`
	}
	if err := json.Unmarshal([]byte(raw), &entry); err != nil || entry.Value == "" {
		return "", false
	}
	if r.now().Sub(time.Unix(entry.FetchedAt, 0)) > cacheTTL {
		return "", false
	}
	return entry.Value, true
}

func (r *Resolver) save(ctx context.Context, key, value string) {
	if r.Cache == nil {
		return
	}
	raw, err := json.Marshal(struct {
		Value     string `json:"value"`
		FetchedAt int64  `json:"fetched_at"`
	}{
		Value:     value,
		FetchedAt: r.now().Unix(),
	})
	if err != nil {
		return
	}
	_ = r.Cache.SaveModelCapability(ctx, key, string(raw))
}

func capabilityKey(route provider.Route, profile settings.Provider) string {
	raw := string(route.Provider) + "\x00" + route.Model + "\x00" + profile.BaseURL + "\x00" + profile.Format
	sum := sha256.Sum256([]byte(raw))
	return "model_capability:" + hex.EncodeToString(sum[:])
}

func providerCandidates(route provider.Route, profile settings.Provider) []string {
	candidates := make([]string, 0, 4)
	parsed, err := url.Parse(strings.TrimSpace(profile.BaseURL))
	if err == nil {
		host := strings.ToLower(parsed.Hostname())
		cleanPath := path.Clean(parsed.Path)
		switch {
		case host == "opencode.ai" && strings.HasPrefix(cleanPath, "/zen/go"):
			candidates = append(candidates, "opencode-go")
		case host == "opencode.ai" && strings.HasPrefix(cleanPath, "/zen"):
			candidates = append(candidates, "opencode")
		case host == "api.openai.com":
			candidates = append(candidates, "openai")
		case host == "api.anthropic.com":
			candidates = append(candidates, "anthropic")
		}
	}
	if id := strings.TrimSpace(string(route.Provider)); id != "" {
		candidates = append(candidates, id)
	}
	if index := strings.LastIndex(route.Model, "/"); index > 0 {
		candidates = append(candidates, strings.TrimSpace(route.Model[:index]))
	}
	return unique(candidates)
}

func matchingReasoningCapabilities(catalog modelsDevCatalog, query string) []capabilityMatch {
	matches := make([]capabilityMatch, 0)
	for providerID, providerEntry := range catalog {
		for modelID, model := range providerEntry.Models {
			rank := modelMatchRank(query, modelID)
			if rank < 0 {
				continue
			}
			value, ok := reasoningEffort(model)
			if !ok {
				continue
			}
			matches = append(matches, capabilityMatch{
				provider: providerID,
				model:    modelID,
				rank:     rank,
				value:    value,
			})
		}
	}
	return matches
}

func preferredMatchValue(matches []capabilityMatch, candidate string) (string, bool) {
	bestRank := int(^uint(0) >> 1)
	value := ""
	found := false
	for _, match := range matches {
		if match.provider != candidate {
			continue
		}
		if match.rank < bestRank {
			bestRank = match.rank
			value = match.value
			found = true
			continue
		}
		if match.rank == bestRank && match.value != value {
			return "", false
		}
	}
	return value, found
}

func unambiguousMatchValue(matches []capabilityMatch) (string, bool) {
	bestRank := int(^uint(0) >> 1)
	value := ""
	found := false
	for _, match := range matches {
		if match.rank > bestRank {
			continue
		}
		if match.rank < bestRank {
			bestRank = match.rank
			value = match.value
			found = true
			continue
		}
		if match.value != value {
			return "", false
		}
	}
	return value, found
}

func modelMatchRank(query, model string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	model = strings.ToLower(strings.TrimSpace(model))
	if query == "" || model == "" {
		return -1
	}
	if query == model {
		return 0
	}
	queryBase := pathBase(query)
	modelBase := pathBase(model)
	if queryBase != modelBase {
		return -1
	}
	family := familyToken(queryBase)
	if family != "" {
		if namespace, _, ok := strings.Cut(model, "/"); ok && namespace == family {
			return 1
		}
		if strings.HasPrefix(modelBase, family+"-") {
			return 2
		}
	}
	return 3
}

func pathBase(value string) string {
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[index+1:]
	}
	return value
}

func familyToken(value string) string {
	if index := strings.IndexByte(value, '-'); index > 0 {
		return value[:index]
	}
	return ""
}

func reasoningEffort(model modelsDevModel) (string, bool) {
	if !model.Reasoning {
		return "", false
	}
	for _, option := range model.ReasoningOptions {
		if option.Type != "effort" || len(option.Values) == 0 {
			continue
		}
		if value := minimumEffort(option.Values); value != "" {
			return value, true
		}
	}
	return "", false
}

func minimumEffort(values []string) string {
	rank := map[string]int{
		"minimal": 0,
		"low":     1,
		"medium":  2,
		"high":    3,
		"max":     4,
	}
	best := ""
	bestRank := 0
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		valueRank, known := rank[value]
		if !known {
			valueRank = len(rank)
		}
		if best == "" || valueRank < bestRank {
			best = value
			bestRank = valueRank
		}
	}
	return best
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
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

var _ ports.ModelCapabilityResolver = (*Resolver)(nil)
