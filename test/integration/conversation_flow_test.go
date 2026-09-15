package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ask"
	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
	"github.com/KevinLeeNJ/ask/internal/platform/clock"
	"github.com/KevinLeeNJ/ask/internal/platform/id"
	"github.com/KevinLeeNJ/ask/internal/provider/registry"
	"github.com/KevinLeeNJ/ask/internal/storage/sqlite"
)

func TestConversationContextAndOneTimeTitleGeneration(t *testing.T) {
	serverState := &chatServerState{}
	server := httptest.NewServer(http.HandlerFunc(serverState.handle))
	defer server.Close()

	repository, err := sqlite.Open(filepath.Join(t.TempDir(), "ask.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	defer repository.Close()

	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        server.URL,
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: 3 * time.Second,
	}
	service := ask.NewService(ask.Dependencies{
		Configs:       staticConfigStore{config: config},
		Secrets:       staticSecretResolver{},
		Providers:     registry.New(server.Client()),
		Conversations: repository,
		Leases:        repository,
		Retention:     repository,
		Routes:        repository,
		Clock:         clock.System{},
		IDs:           id.UUIDv7{},
	}, silentPresenter{})

	first, err := service.Execute(context.Background(), ask.Options{
		Question:        "first question",
		DefaultSystemEN: "system",
		Language:        "en-US",
	})
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first.ConversationID == "" || first.MessageID == "" {
		t.Fatalf("first result = %#v", first)
	}
	waitForTitle(t, repository, conversation.ID(first.ConversationID))

	second, err := service.Execute(context.Background(), ask.Options{
		Question:        "second question",
		DefaultSystemEN: "system",
		Language:        "en-US",
		DisplayCapacity: 1344,
	})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("conversation changed: %s -> %s", first.ConversationID, second.ConversationID)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.titleRequests != 1 {
		t.Fatalf("title requests = %d, want 1", serverState.titleRequests)
	}
	routes, err := repository.RecentRoutes(context.Background(), 10)
	if err != nil {
		t.Fatalf("RecentRoutes() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Provider != "work" || routes[0].Model != "model-a" {
		t.Fatalf("recent routes = %#v", routes)
	}
	if len(serverState.mainRequests) != 2 {
		t.Fatalf("main requests = %d, want 2", len(serverState.mainRequests))
	}
	if len(serverState.mainRequests[0]) != 2 {
		t.Fatalf("first messages = %#v", serverState.mainRequests[0])
	}
	if len(serverState.mainRequests[1]) != 4 {
		t.Fatalf("second messages = %#v", serverState.mainRequests[1])
	}
	if serverState.mainRequests[1][1] != "first question" ||
		serverState.mainRequests[1][2] != "answer-1" ||
		!strings.HasPrefix(serverState.mainRequests[1][3], "second question\n\n[Terminal display budget:") {
		t.Fatalf("second context = %#v", serverState.mainRequests[1])
	}

	messages, err := repository.Messages(context.Background(), conversation.ID(first.ConversationID), 10)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("persisted messages = %d, want 4", len(messages))
	}
	if messages[2].Content != "second question" {
		t.Fatalf("persisted second question = %q", messages[2].Content)
	}
	value, err := repository.Get(context.Background(), conversation.ID(first.ConversationID))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value.Title != "Generated title" || value.TitleSource != conversation.TitleAgent {
		t.Fatalf("conversation title = %#v", value)
	}
}

func TestCancellationPersistsPartialAssistantMessage(t *testing.T) {
	repository, err := sqlite.Open(filepath.Join(t.TempDir(), "ask.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	defer repository.Close()

	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://example.invalid",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Minute,
	}
	chat := &cancelableChat{started: make(chan struct{})}
	presenter := &signalingPresenter{content: make(chan struct{}, 1)}
	service := ask.NewService(ask.Dependencies{
		Configs:       staticConfigStore{config: config},
		Secrets:       staticSecretResolver{},
		Providers:     staticProviderResolver{chat: chat},
		Conversations: repository,
		Leases:        repository,
		Retention:     repository,
		Clock:         clock.System{},
		IDs:           id.UUIDv7{},
	}, presenter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultChannel := make(chan error, 1)
	go func() {
		_, err := service.Execute(ctx, ask.Options{
			Question:        "question",
			DefaultSystemEN: "system",
			Language:        "en-US",
		})
		resultChannel <- err
	}()
	select {
	case <-chat.started:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	select {
	case <-presenter.content:
	case <-time.After(time.Second):
		t.Fatal("partial content was not presented")
	}
	cancel()
	err = <-resultChannel
	if failure.KindOf(err) != failure.KindCanceled {
		t.Fatalf("Execute() error = %v", err)
	}

	conversationValue, err := repository.Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	messages, err := repository.Messages(context.Background(), conversationValue.ID, 10)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[1].Role != "assistant" || messages[1].Status != conversation.StatusPartial || messages[1].Content != "partial" {
		t.Fatalf("assistant message = %#v", messages[1])
	}
}

func TestExistingConversationUsesStoredRoute(t *testing.T) {
	repository, err := sqlite.Open(filepath.Join(t.TempDir(), "ask.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	defer repository.Close()

	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://work.example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: time.Second,
	}
	config.Providers["work-alt"] = settings.Provider{
		Format:         string(provider.FormatAnthropicMessages),
		BaseURL:        "https://alt.example.com",
		DefaultModel:   "model-alt-default",
		EnabledModels:  []string{"model-alt", "model-alt-default"},
		RequestTimeout: time.Second,
	}

	now := time.Now()
	value := conversation.Conversation{
		ID:          "018f0000-0000-7000-8000-000000000099",
		Title:       "existing",
		TitleSource: conversation.TitleProvisional,
		Provider:    "work-alt",
		Model:       "model-alt",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repository.Create(context.Background(), value); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repository.SetCurrent(context.Background(), value.ID); err != nil {
		t.Fatalf("SetCurrent() error = %v", err)
	}

	chat := &routingChat{events: []stream.Event{
		stream.TextDelta{Text: "answer"},
		stream.Completed{},
	}}
	service := ask.NewService(ask.Dependencies{
		Configs:       staticConfigStore{config: config},
		Secrets:       staticSecretResolver{},
		Providers:     staticProviderResolver{chat: chat},
		Conversations: repository,
		Leases:        repository,
		Retention:     repository,
		Clock:         clock.System{},
		IDs:           id.UUIDv7{},
	}, silentPresenter{})

	result, err := service.Execute(context.Background(), ask.Options{
		Question:        "continue",
		DefaultSystemEN: "system",
		Language:        "en-US",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ConversationID != string(value.ID) {
		t.Fatalf("conversation ID = %q, want %q", result.ConversationID, value.ID)
	}
	if len(chat.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(chat.requests))
	}
	route := chat.requests[0].Route
	if route.Provider != "work-alt" || route.Model != "model-alt" {
		t.Fatalf("route = %#v", route)
	}
}

type chatServerState struct {
	mu            sync.Mutex
	mainRequests  [][]string
	titleRequests int
}

func (s *chatServerState) handle(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	body := struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	contents := make([]string, 0, len(body.Messages))
	for _, message := range body.Messages {
		contents = append(contents, message.Content)
	}
	s.mu.Lock()
	if body.Stream {
		s.mainRequests = append(s.mainRequests, contents)
		answer := "answer-" + string(rune('0'+len(s.mainRequests)))
		s.mu.Unlock()
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\""+answer+"\"}}]}\n\n")
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		return
	}
	s.titleRequests++
	s.mu.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(writer, `{"model":"model-a","choices":[{"message":{"content":"Generated title"}}]}`)
}

func waitForTitle(t *testing.T, repository *sqlite.Repository, id conversation.ID) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		value, err := repository.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value.TitleSource == conversation.TitleAgent {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("title was not generated")
}

type staticConfigStore struct {
	config settings.Config
}

func (s staticConfigStore) Load(context.Context) (settings.Config, error) {
	return s.config, nil
}

func (s staticConfigStore) Save(context.Context, settings.Config) error {
	return nil
}

type staticSecretResolver struct{}

func (staticSecretResolver) Resolve(context.Context, provider.ID) (ports.Secret, error) {
	return ports.Secret{Provider: "work", Value: "secret"}, nil
}

type silentPresenter struct{}

func (silentPresenter) Progress(stream.Progress)   {}
func (silentPresenter) ContentDelta(string)        {}
func (silentPresenter) ReasoningDelta(string)      {}
func (silentPresenter) Complete(provider.Response) {}
func (silentPresenter) Fail(error)                 {}

var _ ports.ConfigStore = staticConfigStore{}
var _ ports.SecretResolver = staticSecretResolver{}
var _ ports.Presenter = silentPresenter{}

type cancelableChat struct {
	started chan struct{}
}

func (c *cancelableChat) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Content: "title"}, nil
}

func (c *cancelableChat) Stream(ctx context.Context, request provider.Request) (ports.Stream, error) {
	return &cancelableStream{
		ctx:      ctx,
		started:  c.started,
		provider: request.Route.Provider,
	}, nil
}

type cancelableStream struct {
	ctx      context.Context
	started  chan struct{}
	provider provider.ID
	once     sync.Once
}

func (s *cancelableStream) Recv() (stream.Event, error) {
	first := false
	s.once.Do(func() {
		first = true
		close(s.started)
	})
	if first {
		return stream.TextDelta{Text: "partial"}, nil
	}
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

func (s *cancelableStream) Close() error {
	return nil
}

type staticProviderResolver struct {
	chat ports.ChatProvider
}

func (r staticProviderResolver) Resolve(settings.Provider, ports.Secret) (ports.ResolvedProvider, error) {
	return ports.ResolvedProvider{Chat: r.chat}, nil
}

type signalingPresenter struct {
	silentPresenter
	content chan struct{}
}

func (p *signalingPresenter) ContentDelta(string) {
	select {
	case p.content <- struct{}{}:
	default:
	}
}

var _ ports.ProviderResolver = staticProviderResolver{}
var _ ports.ChatProvider = (*cancelableChat)(nil)
var _ ports.Stream = (*cancelableStream)(nil)

type routingChat struct {
	requests []provider.Request
	events   []stream.Event
}

func (c *routingChat) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Content: "complete"}, nil
}

func (c *routingChat) Stream(_ context.Context, request provider.Request) (ports.Stream, error) {
	c.requests = append(c.requests, request)
	return &routingStream{events: append([]stream.Event(nil), c.events...)}, nil
}

type routingStream struct {
	events []stream.Event
}

func (s *routingStream) Recv() (stream.Event, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *routingStream) Close() error {
	return nil
}

var _ ports.ChatProvider = (*routingChat)(nil)
var _ ports.Stream = (*routingStream)(nil)
