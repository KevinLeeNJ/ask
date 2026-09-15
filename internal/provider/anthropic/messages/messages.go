package messages

import (
	"context"
	"io"
	"net/url"
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

const anthropicVersion = "2023-06-01"

func New(config ports.ProviderHTTPConfig) *Client {
	return &Client{Config: config}
}

func (c *Client) Complete(ctx context.Context, request provider.Request) (provider.Response, error) {
	payload, err := c.payload(request, false)
	if err != nil {
		return provider.Response{}, err
	}

	headers := c.Config.RequestHeaders(request.SessionID)
	headers["anthropic-version"] = anthropicVersion
	headers["Accept"] = "application/json"
	httpRequest, err := transport.NewJSONRequest(ctx, "POST", c.Config.BaseURL, "/v1/messages", payload, headers)
	if err != nil {
		return provider.Response{}, err
	}
	body, err := transport.Do(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
	if err != nil {
		return provider.Response{}, err
	}

	var decoded struct {
		Model   string `json:"model"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
		return provider.Response{}, err
	}
	var content strings.Builder
	var reasoning strings.Builder
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		}
	}
	if strings.TrimSpace(content.String()) == "" {
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
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		Usage: provider.Usage{
			InputTokens:  decoded.Usage.InputTokens,
			OutputTokens: decoded.Usage.OutputTokens,
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
	headers["anthropic-version"] = anthropicVersion
	headers["Accept"] = "text/event-stream"
	httpRequest, err := transport.NewJSONRequest(ctx, "POST", c.Config.BaseURL, "/v1/messages", payload, headers)
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
	headers["anthropic-version"] = anthropicVersion

	afterID := ""
	all := make([]provider.Model, 0)
	for page := 0; page < catalog.MaxPages; page++ {
		suffix := "/v1/models"
		if afterID != "" {
			suffix += "?after_id=" + url.QueryEscape(afterID)
		}
		httpRequest, err := transport.NewJSONRequest(ctx, "GET", c.Config.BaseURL, suffix, nil, headers)
		if err != nil {
			return nil, err
		}
		body, err := transport.Do(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				CreatedAt   string `json:"created_at"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
			return nil, err
		}
		for _, item := range decoded.Data {
			all = append(all, provider.Model{
				ID:          item.ID,
				DisplayName: item.DisplayName,
				CreatedAt:   item.CreatedAt,
			})
		}
		if len(all) >= catalog.MaxModels {
			all = all[:catalog.MaxModels]
			break
		}
		if !decoded.HasMore || decoded.LastID == "" || decoded.LastID == afterID {
			break
		}
		afterID = decoded.LastID
	}
	return catalog.Normalize(all), nil
}

func toMessages(messages []provider.Message) []map[string]string {
	result := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		if message.Role == provider.RoleSystem {
			continue
		}
		result = append(result, map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		})
	}
	return result
}

func (c *Client) payload(request provider.Request, streaming bool) (map[string]any, error) {
	payload := map[string]any{
		"model":      request.Route.Model,
		"messages":   toMessages(request.Messages),
		"max_tokens": request.MaxOutputTokens,
		"stream":     streaming,
	}
	if strings.TrimSpace(request.SystemPrompt) != "" {
		payload["system"] = request.SystemPrompt
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
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
		eventName, data := s.reader.Event()
		var decoded struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := transport.DecodeJSON([]byte(data), &decoded, s.provider); err != nil {
			return nil, err
		}
		kind := decoded.Type
		if kind == "" {
			kind = eventName
		}
		if decoded.Message.Model != "" {
			s.model = decoded.Message.Model
		}
		switch kind {
		case "message_start":
			if decoded.Message.Usage.InputTokens != 0 {
				s.usage.InputTokens = decoded.Message.Usage.InputTokens
				s.queue = append(s.queue, stream.Usage{Usage: s.usage})
			}
		case "content_block_delta":
			switch decoded.Delta.Type {
			case "text_delta":
				if decoded.Delta.Text != "" {
					s.sawText = true
					s.queue = append(s.queue, stream.TextDelta{Text: decoded.Delta.Text})
				}
			case "thinking_delta":
				if decoded.Delta.Thinking != "" {
					s.sawReasoning = true
					s.queue = append(s.queue, stream.ReasoningDelta{Text: decoded.Delta.Thinking})
				}
			}
		case "message_delta":
			if decoded.Usage.InputTokens != 0 {
				s.usage.InputTokens = decoded.Usage.InputTokens
			}
			if decoded.Usage.OutputTokens != 0 {
				s.usage.OutputTokens = decoded.Usage.OutputTokens
			}
			s.queue = append(s.queue, stream.Usage{Usage: s.usage})
		case "message_stop":
			s.completed = true
			s.queue = append(s.queue, stream.Completed{})
		case "error":
			message := decoded.Error.Message
			if message == "" {
				message = "anthropic error"
			}
			return nil, failure.New(
				failure.KindUpstream,
				"upstream.response_failed",
				map[string]string{"provider": string(s.provider), "reason": message},
			)
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

func applyThinking(payload map[string]any, thinking provider.ThinkingRequest) error {
	if !thinking.Explicit || thinking.OmitWhenOff {
		return nil
	}
	if thinking.Enabled {
		if thinking.AnthropicMax <= 0 {
			return failure.New(failure.KindConfig, "reasoning.anthropic_budget_invalid", nil)
		}
		payload["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": thinking.AnthropicMax,
		}
		return nil
	}
	payload["thinking"] = map[string]any{"type": "disabled"}
	return nil
}
