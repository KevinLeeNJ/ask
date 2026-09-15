package provider

import (
	"fmt"
	"sort"
	"strings"
)

type ID string

type Format string

const (
	FormatOpenAIChatCompletions Format = "openai_chat_completions"
	FormatOpenAIResponses       Format = "openai_responses"
	FormatAnthropicMessages     Format = "anthropic_messages"
)

func (f Format) Valid() bool {
	switch f {
	case FormatOpenAIChatCompletions, FormatOpenAIResponses, FormatAnthropicMessages:
		return true
	default:
		return false
	}
}

func (f Format) Path() string {
	switch f {
	case FormatOpenAIChatCompletions:
		return "/v1/chat/completions"
	case FormatOpenAIResponses:
		return "/v1/responses"
	case FormatAnthropicMessages:
		return "/v1/messages"
	default:
		return ""
	}
}

func EnvVar(id ID) string {
	normalized := strings.ToUpper(strings.ReplaceAll(string(id), "-", "_"))
	return normalized + "_API_KEY"
}

type Route struct {
	Provider ID     `json:"provider"`
	Model    string `json:"model"`
}

func (r Route) Validate() error {
	if strings.TrimSpace(string(r.Provider)) == "" {
		return fmt.Errorf("provider is empty")
	}
	if strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("model is empty")
	}
	return nil
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role
	Content string
}

type ThinkingMode string

const (
	ThinkingAuto ThinkingMode = "auto"
	ThinkingOn   ThinkingMode = "on"
	ThinkingOff  ThinkingMode = "off"
)

func (m ThinkingMode) Valid() bool {
	switch m {
	case ThinkingAuto, ThinkingOn, ThinkingOff:
		return true
	default:
		return false
	}
}

type ThinkingRequest struct {
	Explicit     bool
	Enabled      bool
	Field        string
	Value        any
	OmitWhenOff  bool
	AnthropicMax int
}

type Request struct {
	Route           Route
	SessionID       string
	SystemPrompt    string
	Messages        []Message
	Stream          bool
	Temperature     *float64
	MaxOutputTokens int
	Thinking        ThinkingRequest
}

type Usage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}

type Response struct {
	Content          string
	ReasoningContent string
	Usage            Usage
	Provider         ID
	Model            string
}

type Model struct {
	ID          string
	DisplayName string
	CreatedAt   string
}

func NormalizeModels(models []Model) []Model {
	seen := make(map[string]Model, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		existing, ok := seen[model.ID]
		if !ok || existing.DisplayName == "" {
			existing = model
		}
		if existing.CreatedAt == "" {
			existing.CreatedAt = model.CreatedAt
		}
		seen[model.ID] = existing
	}
	result := make([]Model, 0, len(seen))
	for _, model := range seen {
		result = append(result, model)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}
