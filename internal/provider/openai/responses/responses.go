package responses

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
	httpRequest, err := transport.NewJSONRequest(ctx, "POST", c.Config.BaseURL, "/v1/responses", payload, headers)
	if err != nil {
		return provider.Response{}, err
	}
	body, err := transport.Do(ctx, c.Config.Client, httpRequest, c.Config.ProviderID)
	if err != nil {
		return provider.Response{}, err
	}

	var decoded struct {
		Model      string `json:"model"`
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens   int `json:"input_tokens"`
			OutputTokens  int `json:"output_tokens"`
			OutputDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
		return provider.Response{}, err
	}
	content := strings.TrimSpace(decoded.OutputText)
	if content == "" {
		var builder strings.Builder
		for _, output := range decoded.Output {
			for _, block := range output.Content {
				if block.Type == "output_text" || block.Type == "text" {
					builder.WriteString(block.Text)
				}
			}
		}
		content = builder.String()
	}
	if strings.TrimSpace(content) == "" {
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
		Content: content,
		Usage: provider.Usage{
			InputTokens:     decoded.Usage.InputTokens,
			OutputTokens:    decoded.Usage.OutputTokens,
			ReasoningTokens: decoded.Usage.OutputDetails.ReasoningTokens,
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
	httpRequest, err := transport.NewJSONRequest(ctx, "POST", c.Config.BaseURL, "/v1/responses", payload, headers)
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
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := transport.DecodeJSON(body, &decoded, c.Config.ProviderID); err != nil {
		return nil, err
	}
	models := make([]provider.Model, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		models = append(models, provider.Model{ID: item.ID, DisplayName: item.OwnedBy})
	}
	return catalog.Normalize(models), nil
}

func toInput(messages []provider.Message) []map[string]string {
	input := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		if message.Role == provider.RoleSystem {
			continue
		}
		input = append(input, map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		})
	}
	return input
}

func (c *Client) payload(request provider.Request, streaming bool) (map[string]any, error) {
	payload := map[string]any{
		"model":  request.Route.Model,
		"input":  toInput(request.Messages),
		"stream": streaming,
	}
	if strings.TrimSpace(request.SystemPrompt) != "" {
		payload["instructions"] = request.SystemPrompt
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.MaxOutputTokens > 0 {
		payload["max_output_tokens"] = request.MaxOutputTokens
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
		if strings.TrimSpace(data) == "[DONE]" {
			s.completed = true
			return stream.Completed{}, nil
		}
		var decoded struct {
			Type     string `json:"type"`
			Model    string `json:"model"`
			Delta    string `json:"delta"`
			Text     string `json:"text"`
			Response struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens   int `json:"input_tokens"`
					OutputTokens  int `json:"output_tokens"`
					OutputDetails struct {
						ReasoningTokens int `json:"reasoning_tokens"`
					} `json:"output_tokens_details"`
				} `json:"usage"`
			} `json:"response"`
			Usage struct {
				InputTokens   int `json:"input_tokens"`
				OutputTokens  int `json:"output_tokens"`
				OutputDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
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
		if decoded.Response.Model != "" {
			s.model = decoded.Response.Model
		}
		if decoded.Model != "" {
			s.model = decoded.Model
		}
		switch kind {
		case "response.output_text.delta":
			text := decoded.Delta
			if text == "" {
				text = decoded.Text
			}
			if text != "" {
				s.sawText = true
				s.queue = append(s.queue, stream.TextDelta{Text: text})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			text := decoded.Delta
			if text == "" {
				text = decoded.Text
			}
			if text != "" {
				s.sawReasoning = true
				s.queue = append(s.queue, stream.ReasoningDelta{Text: text})
			}
		case "response.completed":
			usage := decoded.Response.Usage
			s.usage = provider.Usage{
				InputTokens:     usage.InputTokens,
				OutputTokens:    usage.OutputTokens,
				ReasoningTokens: usage.OutputDetails.ReasoningTokens,
			}
			s.queue = append(s.queue, stream.Usage{Usage: s.usage}, stream.Completed{})
			s.completed = true
		case "response.failed":
			message := decoded.Error.Message
			if message == "" {
				message = "response.failed"
			}
			return nil, failure.New(
				failure.KindUpstream,
				"upstream.response_failed",
				map[string]string{"provider": string(s.provider), "reason": message},
			)
		}
		if decoded.Usage.InputTokens != 0 || decoded.Usage.OutputTokens != 0 {
			s.usage = provider.Usage{
				InputTokens:     decoded.Usage.InputTokens,
				OutputTokens:    decoded.Usage.OutputTokens,
				ReasoningTokens: decoded.Usage.OutputDetails.ReasoningTokens,
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

func applyThinking(payload map[string]any, thinking provider.ThinkingRequest) error {
	if !thinking.Explicit || thinking.OmitWhenOff {
		return nil
	}
	field := thinking.Field
	if field == "" || field == "reasoning_effort" {
		field = "reasoning"
	}
	if field == "reasoning" {
		payload["reasoning"] = map[string]any{"effort": thinking.Value}
		return nil
	}
	payload[field] = thinking.Value
	return nil
}
