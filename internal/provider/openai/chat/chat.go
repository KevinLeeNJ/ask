package chat

import (
	"context"
	"io"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
	"github.com/KevinLeeNJ/ask/internal/provider/catalog"
	"github.com/KevinLeeNJ/ask/internal/provider/transport"
)

type Client struct {
	Config ports.ProviderHTTPConfig
}

func New(config ports.ProviderHTTPConfig) *Client {
	return &Client{Config: config}
}

func (c *Client) Complete(ctx context.Context, request provider.Request) (provider.Response, error) {
	payload, err := c.payload(request, false)
	if err != nil {
		return provider.Response{}, err
	}

	headers := c.Config.RequestHeaders(request.SessionID)
	headers["Accept"] = "application/json"
	httpRequest, err := transport.NewJSONRequest(
		ctx,
		"POST",
		c.Config.BaseURL,
		"/v1/chat/completions",
		payload,
		headers,
	)
	if err != nil {
		return provider.Response{}, err
	}
	body, err := transport.Do(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
	if err != nil {
		return provider.Response{}, err
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens      int `json:"prompt_tokens"`
			CompletionTokens  int `json:"completion_tokens"`
			CompletionDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
		return provider.Response{}, err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return provider.Response{}, failure.New(
			failure.KindProtocol,
			"upstream.empty_response",
			map[string]string{"provider": string(c.Config.ProviderID)},
		)
	}
	model := decoded.Model
	if model == "" {
		model = request.Route.Model
	}
	return provider.Response{
		Content:          decoded.Choices[0].Message.Content,
		ReasoningContent: decoded.Choices[0].Message.ReasoningContent,
		Usage: provider.Usage{
			InputTokens:     decoded.Usage.PromptTokens,
			OutputTokens:    decoded.Usage.CompletionTokens,
			ReasoningTokens: decoded.Usage.CompletionDetails.ReasoningTokens,
		},
		Provider: c.Config.ProviderID,
		Model:    model,
	}, nil
}

func (c *Client) Stream(ctx context.Context, request provider.Request) (ports.Stream, error) {
	payload, err := c.payload(request, true)
	if err != nil {
		return nil, err
	}
	headers := c.Config.RequestHeaders(request.SessionID)
	headers["Accept"] = "text/event-stream"
	httpRequest, err := transport.NewJSONRequest(
		ctx,
		"POST",
		c.Config.BaseURL,
		"/v1/chat/completions",
		payload,
		headers,
	)
	if err != nil {
		return nil, err
	}
	body, err := transport.OpenStream(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
	if err != nil {
		return nil, err
	}
	return &eventStream{
		body:     body,
		reader:   transport.NewSSEReader(body),
		model:    request.Route.Model,
		provider: c.Config.ProviderID,
	}, nil
}

func (c *Client) ListModels(ctx context.Context, route provider.Route) ([]provider.Model, error) {
	headers := c.Config.RequestHeaders("")
	headers["Accept"] = "application/json"
	httpRequest, err := transport.NewJSONRequest(ctx, "GET", c.Config.BaseURL, "/v1/models", nil, headers)
	if err != nil {
		return nil, err
	}
	body, err := transport.Do(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
		return nil, err
	}
	models := make([]provider.Model, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		models = append(models, provider.Model{
			ID:          item.ID,
			DisplayName: item.OwnedBy,
		})
	}
	return catalog.Normalize(models), nil
}

func toMessages(request provider.Request) []map[string]string {
	messages := make([]map[string]string, 0, len(request.Messages)+1)
	if strings.TrimSpace(request.SystemPrompt) != "" {
		messages = append(messages, map[string]string{
			"role":    string(provider.RoleSystem),
			"content": request.SystemPrompt,
		})
	}
	for _, message := range request.Messages {
		messages = append(messages, map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		})
	}
	return messages
}

func (c *Client) payload(request provider.Request, streaming bool) (map[string]any, error) {
	payload := map[string]any{
		"model":    request.Route.Model,
		"messages": toMessages(request),
		"stream":   streaming,
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.MaxOutputTokens > 0 {
		payload["max_tokens"] = request.MaxOutputTokens
	}
	if err := applyThinking(payload, request.Thinking); err != nil {
		return nil, err
	}
	return payload, nil
}

type eventStream struct {
	body         io.Closer
	reader       *transport.SSEReader
	queue        []stream.Event
	model        string
	provider     provider.ID
	usage        provider.Usage
	sawText      bool
	sawReasoning bool
	completed    bool
	closed       bool
}

func (s *eventStream) Recv() (stream.Event, error) {
	if len(s.queue) > 0 {
		return s.pop(), nil
	}
	if s.completed {
		return nil, io.EOF
	}

	for s.reader.Next() {
		_, data := s.reader.Event()
		if strings.TrimSpace(data) == "[DONE]" {
			s.completed = true
			return stream.Completed{}, nil
		}
		var decoded struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
				} `json:"delta"`
				Message struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens      int `json:"prompt_tokens"`
				CompletionTokens  int `json:"completion_tokens"`
				CompletionDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		if err := transport.DecodeJSON([]byte(data), &decoded, s.provider); err != nil {
			return nil, err
		}
		if decoded.Model != "" {
			s.model = decoded.Model
		}
		for _, choice := range decoded.Choices {
			reasoning := choice.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = choice.Delta.Reasoning
			}
			if reasoning == "" {
				reasoning = choice.Message.ReasoningContent
			}
			if reasoning != "" {
				s.sawReasoning = true
				s.queue = append(s.queue, stream.ReasoningDelta{Text: reasoning})
			}
			content := choice.Delta.Content
			if content == "" {
				content = choice.Message.Content
			}
			if content != "" {
				s.sawText = true
				s.queue = append(s.queue, stream.TextDelta{Text: content})
			}
		}
		if decoded.Usage.PromptTokens != 0 || decoded.Usage.CompletionTokens != 0 || decoded.Usage.CompletionDetails.ReasoningTokens != 0 {
			s.usage = provider.Usage{
				InputTokens:     firstNonZero(decoded.Usage.PromptTokens, s.usage.InputTokens),
				OutputTokens:    firstNonZero(decoded.Usage.CompletionTokens, s.usage.OutputTokens),
				ReasoningTokens: firstNonZero(decoded.Usage.CompletionDetails.ReasoningTokens, s.usage.ReasoningTokens),
			}
			s.queue = append(s.queue, stream.Usage{Usage: s.usage})
		}
		if len(s.queue) > 0 {
			return s.pop(), nil
		}
	}
	if err := s.reader.Err(); err != nil {
		return nil, failure.Wrap(
			failure.KindProtocol,
			"protocol.stream_read_failed",
			map[string]string{"provider": string(s.provider), "reason": err.Error()},
			err,
		)
	}
	if !s.sawText && !s.sawReasoning {
		return nil, failure.New(
			failure.KindProtocol,
			"upstream.empty_response",
			map[string]string{"provider": string(s.provider)},
		)
	}
	s.completed = true
	return stream.Completed{}, nil
}

func (s *eventStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func (s *eventStream) pop() stream.Event {
	event := s.queue[0]
	s.queue = s.queue[1:]
	return event
}

func firstNonZero(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func applyThinking(payload map[string]any, thinking provider.ThinkingRequest) error {
	if !thinking.Explicit || thinking.OmitWhenOff {
		return nil
	}
	field := thinking.Field
	if field == "" {
		field = "reasoning_effort"
	}
	if thinking.Enabled {
		payload[field] = thinking.Value
	} else {
		payload[field] = thinking.Value
	}
	return nil
}
