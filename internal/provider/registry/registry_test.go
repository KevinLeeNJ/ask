package registry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	streampkg "github.com/KevinLeeNJ/ask/internal/domain/stream"
)

func TestResolveAndCompleteOpenAIProtocols(t *testing.T) {
	tests := []struct {
		name     string
		format   provider.Format
		path     string
		response string
		assert   func(t *testing.T, body map[string]any)
	}{
		{
			name:   "chat completions",
			format: provider.FormatOpenAIChatCompletions,
			path:   "/v1/chat/completions",
			response: `{
				"model":"model-a",
				"choices":[{"message":{"content":"chat answer","reasoning_content":"reasoning"}}],
				"usage":{"prompt_tokens":11,"completion_tokens":7,"completion_tokens_details":{"reasoning_tokens":3}}
			}`,
			assert: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["stream"] != false {
					t.Fatalf("stream = %#v", body["stream"])
				}
				messages, ok := body["messages"].([]any)
				if !ok || len(messages) != 2 {
					t.Fatalf("messages = %#v", body["messages"])
				}
			},
		},
		{
			name:   "responses",
			format: provider.FormatOpenAIResponses,
			path:   "/v1/responses",
			response: `{
				"model":"model-a",
				"output_text":"responses answer",
				"usage":{"input_tokens":12,"output_tokens":8,"output_tokens_details":{"reasoning_tokens":4}}
			}`,
			assert: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["instructions"] != "system" {
					t.Fatalf("instructions = %#v", body["instructions"])
				}
				if _, ok := body["input"].([]any); !ok {
					t.Fatalf("input = %#v", body["input"])
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Authorization") != "Bearer secret" {
					t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
				}
				switch request.URL.Path {
				case test.path:
					body := decodeBody(t, request)
					test.assert(t, body)
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, test.response)
				case "/v1/models":
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, `{"data":[{"id":"model-b"},{"id":"model-a"}]}`)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			registry := New(server.Client())
			profile := settings.Provider{
				Format:         string(test.format),
				BaseURL:        server.URL,
				DefaultModel:   "model-a",
				EnabledModels:  []string{"model-a"},
				RequestTimeout: time.Second,
			}
			resolved, err := registry.Resolve(profile, ports.Secret{Value: "secret", Provider: "work"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			response, err := resolved.Chat.Complete(context.Background(), provider.Request{
				Route:           provider.Route{Provider: "work", Model: "model-a"},
				SystemPrompt:    "system",
				Messages:        []provider.Message{{Role: provider.RoleUser, Content: "question"}},
				MaxOutputTokens: 100,
			})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if !strings.Contains(response.Content, "answer") {
				t.Fatalf("content = %q", response.Content)
			}
			if response.Usage.InputTokens == 0 || response.Usage.OutputTokens == 0 {
				t.Fatalf("usage = %#v", response.Usage)
			}

			models, err := resolved.Catalog.ListModels(context.Background(), provider.Route{Provider: "work", Model: "model-a"})
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}
			if len(models) != 2 {
				t.Fatalf("models = %#v", models)
			}
		})
	}
}

func TestResolveAndCompleteAnthropicMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "secret" {
			t.Errorf("x-api-key = %q", request.Header.Get("x-api-key"))
		}
		if request.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q", request.Header.Get("anthropic-version"))
		}
		switch request.URL.Path {
		case "/v1/messages":
			body := decodeBody(t, request)
			if body["system"] != "system" {
				t.Fatalf("system = %#v", body["system"])
			}
			if body["max_tokens"] != float64(100) {
				t.Fatalf("max_tokens = %#v", body["max_tokens"])
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{
				"model":"model-a",
				"content":[{"type":"thinking","thinking":"reasoning"},{"type":"text","text":"anthropic answer"}],
				"usage":{"input_tokens":13,"output_tokens":9}
			}`)
		case "/v1/models":
			writer.Header().Set("Content-Type", "application/json")
			if request.URL.Query().Get("after_id") == "" {
				_, _ = io.WriteString(writer, `{"data":[{"id":"model-a","display_name":"A"}],"has_more":true,"last_id":"model-a"}`)
				return
			}
			_, _ = io.WriteString(writer, `{"data":[{"id":"model-b","display_name":"B"}],"has_more":false}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	registry := New(server.Client())
	profile := settings.Provider{
		Format:         string(provider.FormatAnthropicMessages),
		BaseURL:        server.URL,
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Second,
	}
	resolved, err := registry.Resolve(profile, ports.Secret{Value: "secret", Provider: "work"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	response, err := resolved.Chat.Complete(context.Background(), provider.Request{
		Route:           provider.Route{Provider: "work", Model: "model-a"},
		SystemPrompt:    "system",
		Messages:        []provider.Message{{Role: provider.RoleUser, Content: "question"}},
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "anthropic answer" || response.ReasoningContent != "reasoning" {
		t.Fatalf("response = %#v", response)
	}
	models, err := resolved.Catalog.ListModels(context.Background(), provider.Route{Provider: "work", Model: "model-a"})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
}

func TestOpenCodeGoRequestHeaders(t *testing.T) {
	client := &recordingHTTPDoer{body: `{
		"model":"model-a",
		"choices":[{"message":{"content":"answer"}}],
		"usage":{"prompt_tokens":5,"completion_tokens":3}
	}`}
	registry := New(client)
	profile := settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://opencode.ai/zen/go",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Second,
	}
	resolved, err := registry.Resolve(profile, ports.Secret{Value: "secret", Provider: "opencode-go"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	_, err = resolved.Chat.Complete(context.Background(), provider.Request{
		Route:           provider.Route{Provider: "opencode-go", Model: "model-a"},
		SessionID:       "conversation-a",
		Messages:        []provider.Message{{Role: provider.RoleUser, Content: "question"}},
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if client.request == nil {
		t.Fatal("request was not recorded")
	}
	if got := client.request.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := client.request.Header.Get("x-opencode-session"); got != "conversation-a" {
		t.Fatalf("x-opencode-session = %q", got)
	}
	if got := client.request.Header.Get("User-Agent"); !strings.HasPrefix(got, "ask/") {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestSessionIDHeaderForOpenCodeGo(t *testing.T) {
	tests := map[string]string{
		"https://opencode.ai/zen/go":              "x-opencode-session",
		"https://opencode.ai/zen/go/":             "x-opencode-session",
		"https://opencode.ai/zen/go/proxy":        "x-opencode-session",
		"https://opencode.ai/zen":                 "",
		"https://example.com/zen/go":              "",
		"https://opencode.ai.evil.example/zen/go": "",
		"not a url": "",
	}
	for baseURL, want := range tests {
		if got := sessionIDHeader(baseURL); got != want {
			t.Errorf("sessionIDHeader(%q) = %q, want %q", baseURL, got, want)
		}
	}
}

func TestStreamProtocols(t *testing.T) {
	tests := []struct {
		name       string
		format     provider.Format
		path       string
		streamBody string
	}{
		{
			name:   "chat completions",
			format: provider.FormatOpenAIChatCompletions,
			path:   "/v1/chat/completions",
			streamBody: "" +
				"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think-\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"one\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3}}\n\n" +
				"data: [DONE]\n\n",
		},
		{
			name:   "responses",
			format: provider.FormatOpenAIResponses,
			path:   "/v1/responses",
			streamBody: "" +
				"event: response.reasoning_summary_text.delta\n" +
				"data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"think\"}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
				"event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"model-a\",\"usage\":{\"input_tokens\":5,\"output_tokens\":3}}}\n\n",
		},
		{
			name:   "anthropic messages",
			format: provider.FormatAnthropicMessages,
			path:   "/v1/messages",
			streamBody: "" +
				"event: message_start\n" +
				"data: {\"type\":\"message_start\",\"message\":{\"model\":\"model-a\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"think\"}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello \"}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"world\"}}\n\n" +
				"event: message_delta\n" +
				"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n" +
				"event: message_stop\n" +
				"data: {\"type\":\"message_stop\"}\n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					http.NotFound(writer, request)
					return
				}
				if request.Header.Get("Accept") != "text/event-stream" {
					t.Errorf("Accept = %q", request.Header.Get("Accept"))
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, test.streamBody)
			}))
			defer server.Close()

			profile := settings.Provider{
				Format:         string(test.format),
				BaseURL:        server.URL,
				DefaultModel:   "model-a",
				EnabledModels:  []string{"model-a"},
				RequestTimeout: time.Second,
			}
			resolved, err := New(server.Client()).Resolve(profile, ports.Secret{Provider: "work"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			stream, err := resolved.Chat.Stream(context.Background(), provider.Request{
				Route:           provider.Route{Provider: "work", Model: "model-a"},
				Messages:        []provider.Message{{Role: provider.RoleUser, Content: "question"}},
				MaxOutputTokens: 100,
			})
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			defer stream.Close()

			events := collectEvents(t, stream)
			var reasoning, content strings.Builder
			var usage provider.Usage
			completed := false
			for _, event := range events {
				switch typed := event.(type) {
				case streampkg.ReasoningDelta:
					reasoning.WriteString(typed.Text)
				case streampkg.TextDelta:
					content.WriteString(typed.Text)
				case streampkg.Usage:
					usage = typed.Usage
				case streampkg.Completed:
					completed = true
				}
			}
			if content.String() != "hello world" {
				t.Fatalf("content = %q, events = %#v", content.String(), events)
			}
			if reasoning.String() != "think-one" && reasoning.String() != "think" {
				t.Fatalf("reasoning = %q", reasoning.String())
			}
			if usage.InputTokens != 5 || usage.OutputTokens != 3 {
				t.Fatalf("usage = %#v", usage)
			}
			if !completed {
				t.Fatal("missing completed event")
			}
		})
	}
}

type recordingHTTPDoer struct {
	body    string
	request *http.Request
}

func (d *recordingHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.request = request
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(d.body)),
		Request:    request,
	}, nil
}

func collectEvents(t *testing.T, stream ports.Stream) []streampkg.Event {
	t.Helper()
	events := make([]streampkg.Event, 0)
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return events
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
		events = append(events, event)
	}
}

func decodeBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	defer request.Body.Close()
	body := map[string]any{}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}
