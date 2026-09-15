package command

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/auth"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/provider/registry"
)

func TestHelpDoesNotRequireConfiguration(t *testing.T) {
	store := &fakeStore{loadErr: failure.New(failure.KindConfig, "config.not_found", nil)}
	var stdout, stderr bytes.Buffer
	code := NewRunner(Dependencies{Configs: store}).Run(
		context.Background(),
		[]string{"--help"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if store.loads != 0 {
		t.Fatalf("help loaded config %d times", store.loads)
	}
}

func TestSeparatorErrorHappensBeforeConfigLoad(t *testing.T) {
	store := &fakeStore{}
	var stdout, stderr bytes.Buffer
	code := NewRunner(Dependencies{Configs: store}).Run(
		context.Background(),
		[]string{"--", "--model", "是什么"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 2 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if store.loads != 0 {
		t.Fatalf("config loaded %d times", store.loads)
	}
	if !strings.Contains(stderr.String(), `ask "-- --model" 是什么`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunJoinsQuestionAndWritesAnswerToStdout(t *testing.T) {
	var gotPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		body := struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Messages) < 2 {
			t.Fatalf("messages = %#v", body.Messages)
		}
		gotPrompt = body.Messages[len(body.Messages)-1].Content
		if body.Stream {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"secret reasoning\"}}]}\n\n")
			_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
			_, _ = io.WriteString(writer, "data: [DONE]\n\n")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
			"model":"model-a",
			"choices":[{"message":{"content":"answer"}}],
			"usage":{"prompt_tokens":5,"completion_tokens":2}
		}`)
	}))
	defer server.Close()
	t.Setenv("WORK_API_KEY", "secret")

	config := validConfig(server.URL)
	store := &fakeStore{config: config}
	var stdout, stderr bytes.Buffer
	code := NewRunner(Dependencies{
		Configs:  store,
		Secrets:  auth.EnvironmentResolver{},
		Registry: registry.New(server.Client()),
	}).Run(
		context.Background(),
		[]string{"--lang", "en-US", "hello", "world"},
		strings.NewReader("stdin context"),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if gotPrompt != "hello world\n\nstdin context" {
		t.Fatalf("prompt = %q", gotPrompt)
	}
	if stdout.String() != "answer\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "provider=work") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "secret reasoning") {
		t.Fatalf("non-TTY default unexpectedly rendered reasoning: %q", stderr.String())
	}
}

func TestJSONModeKeepsStdoutMachineReadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"reasoning\"}}]}\n\n")
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"# answer\"}}]}\n\n")
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()
	t.Setenv("WORK_API_KEY", "secret")

	var stdout, stderr bytes.Buffer
	code := NewRunner(Dependencies{
		Configs:  &fakeStore{config: validConfig(server.URL)},
		Secrets:  auth.EnvironmentResolver{},
		Registry: registry.New(server.Client()),
	}).Run(
		context.Background(),
		[]string{"--json", "--show-thinking", "hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	if decoded["content"] != "# answer" {
		t.Fatalf("decoded = %#v", decoded)
	}
	reasoning, ok := decoded["reasoning"].(map[string]any)
	if !ok || reasoning["content"] != "reasoning" {
		t.Fatalf("reasoning = %#v", decoded["reasoning"])
	}
}

type fakeStore struct {
	config  settings.Config
	loadErr error
	loads   int
}

func (s *fakeStore) Load(context.Context) (settings.Config, error) {
	s.loads++
	if s.loadErr != nil {
		return settings.Config{}, s.loadErr
	}
	return s.config, nil
}

func (s *fakeStore) Save(context.Context, settings.Config) error {
	return nil
}

func validConfig(baseURL string) settings.Config {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIChatCompletions),
		BaseURL:        baseURL,
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: 2 * time.Second,
	}
	return config
}

var _ ports.ConfigStore = (*fakeStore)(nil)
