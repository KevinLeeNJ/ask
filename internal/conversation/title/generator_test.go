package title

import (
	"context"
	"testing"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

func TestGeneratorUsesNonStreamingRequest(t *testing.T) {
	client := &captureClient{response: provider.Response{Content: "Title: generated title"}}
	generator := Generator{
		Client:    client,
		Route:     provider.Route{Provider: "work", Model: "model-a"},
		SessionID: "conversation-a",
	}
	got, err := generator.Generate(context.Background(), "first question")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "generated title" {
		t.Fatalf("title = %q", got)
	}
	if client.request.Stream || len(client.request.Messages) != 1 || client.request.Messages[0].Content != "first question" {
		t.Fatalf("request = %#v", client.request)
	}
	if !client.request.Thinking.Explicit || !client.request.Thinking.OmitWhenOff {
		t.Fatalf("thinking = %#v", client.request.Thinking)
	}
	if client.request.SessionID != "conversation-a" {
		t.Fatalf("session ID = %q", client.request.SessionID)
	}
}

type captureClient struct {
	request  provider.Request
	response provider.Response
}

func (c *captureClient) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	c.request = request
	return c.response, nil
}

func (c *captureClient) Stream(context.Context, provider.Request) (ports.Stream, error) {
	panic("unexpected Stream call")
}
