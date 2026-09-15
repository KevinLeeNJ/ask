package ask

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
)

func TestExecuteStreamSeparatesReasoningAndContent(t *testing.T) {
	config := testConfig()
	chat := &fakeChat{
		stream: &fakeStream{events: []stream.Event{
			stream.ReasoningDelta{Text: "think"},
			stream.TextDelta{Text: "hello "},
			stream.TextDelta{Text: "world"},
			stream.Usage{Usage: provider.Usage{InputTokens: 5, OutputTokens: 2}},
			stream.Completed{},
		}},
	}
	presenter := &recordingPresenter{}
	service := NewService(Dependencies{
		Configs:   &staticStore{config: config},
		Secrets:   staticSecrets{},
		Providers: staticResolver{chat: chat},
	}, presenter)

	result, err := service.Execute(context.Background(), Options{
		Question:        "question",
		DefaultSystemEN: "system",
		Language:        "en-US",
		NoStream:        false,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.streamCalls != 1 || chat.completeCalls != 0 {
		t.Fatalf("stream=%d complete=%d", chat.streamCalls, chat.completeCalls)
	}
	if result.Response.Content != "hello world" || result.Response.ReasoningContent != "think" {
		t.Fatalf("result = %#v", result.Response)
	}
	if !reflect.DeepEqual(presenter.content, []string{"hello ", "world"}) {
		t.Fatalf("content deltas = %#v", presenter.content)
	}
	if !reflect.DeepEqual(presenter.reasoning, []string{"think"}) {
		t.Fatalf("reasoning deltas = %#v", presenter.reasoning)
	}
	wantStages := []stream.ProgressStage{
		stream.StagePreparing,
		stream.StageConnecting,
		stream.StageWaiting,
		stream.StageThinking,
		stream.StageGenerating,
	}
	if !reflect.DeepEqual(presenter.stages, wantStages) {
		t.Fatalf("stages = %#v, want %#v", presenter.stages, wantStages)
	}
}

func TestExecuteNoStreamUsesComplete(t *testing.T) {
	chat := &fakeChat{completeResponse: provider.Response{
		Content:  "complete",
		Provider: "work",
		Model:    "model-a",
	}}
	service := NewService(Dependencies{
		Configs:   &staticStore{config: testConfig()},
		Secrets:   staticSecrets{},
		Providers: staticResolver{chat: chat},
	}, &recordingPresenter{})
	result, err := service.Execute(context.Background(), Options{
		Question:        "question",
		DefaultSystemEN: "system",
		Language:        "en-US",
		NoStream:        true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.completeCalls != 1 || chat.streamCalls != 0 {
		t.Fatalf("stream=%d complete=%d", chat.streamCalls, chat.completeCalls)
	}
	if result.Response.Content != "complete" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteRepeatsProgressDuringLongWait(t *testing.T) {
	chat := &fakeChat{stream: &fakeStream{
		delay: 1100 * time.Millisecond,
		events: []stream.Event{
			stream.TextDelta{Text: "answer"},
			stream.Completed{},
		},
	}}
	presenter := &recordingPresenter{}
	service := NewService(Dependencies{
		Configs:   &staticStore{config: testConfig()},
		Secrets:   staticSecrets{},
		Providers: staticResolver{chat: chat},
	}, presenter)
	if _, err := service.Execute(context.Background(), Options{
		Question:        "question",
		DefaultSystemEN: "system",
		Language:        "en-US",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	waiting := 0
	for _, stage := range presenter.stages {
		if stage == stream.StageWaiting {
			waiting++
		}
	}
	if waiting < 2 {
		t.Fatalf("waiting progress was not refreshed: %#v", presenter.stages)
	}
}

func TestExecuteTreatsReasoningWithoutAnswerAsEmptyResponse(t *testing.T) {
	chat := &fakeChat{stream: &fakeStream{events: []stream.Event{
		stream.ReasoningDelta{Text: "thinking only"},
		stream.Completed{},
	}}}
	service := NewService(Dependencies{
		Configs:   &staticStore{config: testConfig()},
		Secrets:   staticSecrets{},
		Providers: staticResolver{chat: chat},
	}, &recordingPresenter{})
	_, err := service.Execute(context.Background(), Options{
		Question:        "question",
		DefaultSystemEN: "system",
		Language:        "en-US",
	})
	if err == nil || failure.KindOf(err) != failure.KindProtocol {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteAppendsDisplayBudgetToPromptEnd(t *testing.T) {
	chat := &fakeChat{stream: &fakeStream{events: []stream.Event{
		stream.TextDelta{Text: "answer"},
		stream.Completed{},
	}}}
	service := NewService(Dependencies{
		Configs:   &staticStore{config: testConfig()},
		Secrets:   staticSecrets{},
		Providers: staticResolver{chat: chat},
	}, &recordingPresenter{})

	if _, err := service.Execute(context.Background(), Options{
		Question:        "question",
		DefaultSystemEN: "system",
		Language:        "en-US",
		DisplayCapacity: 1344,
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(chat.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(chat.requests))
	}
	request := chat.requests[0]
	if request.SystemPrompt != "system" {
		t.Fatalf("system prompt = %q", request.SystemPrompt)
	}
	last := request.Messages[len(request.Messages)-1].Content
	if !strings.HasPrefix(last, "question\n\n[Terminal display budget:") ||
		!strings.Contains(last, "1344 character cells") {
		t.Fatalf("last message = %q", last)
	}
}

type staticStore struct {
	config settings.Config
}

func (s *staticStore) Load(context.Context) (settings.Config, error) {
	return s.config, nil
}

func (s *staticStore) Save(context.Context, settings.Config) error {
	return nil
}

type staticSecrets struct{}

func (staticSecrets) Resolve(context.Context, provider.ID) (ports.Secret, error) {
	return ports.Secret{Provider: "work"}, nil
}

type staticResolver struct {
	chat ports.ChatProvider
}

func (r staticResolver) Resolve(settings.Provider, ports.Secret) (ports.ResolvedProvider, error) {
	return ports.ResolvedProvider{Chat: r.chat}, nil
}

type fakeChat struct {
	stream           ports.Stream
	completeResponse provider.Response
	requests         []provider.Request
	streamCalls      int
	completeCalls    int
}

func (c *fakeChat) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	c.completeCalls++
	c.requests = append(c.requests, request)
	return c.completeResponse, nil
}

func (c *fakeChat) Stream(_ context.Context, request provider.Request) (ports.Stream, error) {
	c.streamCalls++
	c.requests = append(c.requests, request)
	return c.stream, nil
}

type fakeStream struct {
	events []stream.Event
	delay  time.Duration
	once   bool
}

func (s *fakeStream) Recv() (stream.Event, error) {
	if s.delay > 0 && !s.once {
		s.once = true
		time.Sleep(s.delay)
	}
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *fakeStream) Close() error {
	return nil
}

type recordingPresenter struct {
	stages    []stream.ProgressStage
	content   []string
	reasoning []string
	complete  provider.Response
	failed    error
}

func (p *recordingPresenter) Progress(event stream.Progress) {
	p.stages = append(p.stages, event.Stage)
}

func (p *recordingPresenter) ContentDelta(text string) {
	p.content = append(p.content, text)
}

func (p *recordingPresenter) ReasoningDelta(text string) {
	p.reasoning = append(p.reasoning, text)
}

func (p *recordingPresenter) Complete(response provider.Response) {
	p.complete = response
}

func (p *recordingPresenter) Fail(err error) {
	p.failed = err
}

func testConfig() settings.Config {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        "https://example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: 5 * time.Second,
	}
	return config
}

var _ ports.ConfigStore = (*staticStore)(nil)
var _ ports.SecretResolver = staticSecrets{}
var _ ports.ProviderResolver = staticResolver{}
var _ ports.ChatProvider = (*fakeChat)(nil)
var _ ports.Stream = (*fakeStream)(nil)
var _ ports.Presenter = (*recordingPresenter)(nil)
