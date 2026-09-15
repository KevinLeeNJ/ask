package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	xterm "github.com/charmbracelet/x/term"

	"github.com/KevinLeeNJ/ask/internal/application/ask"
	"github.com/KevinLeeNJ/ask/internal/application/configure"
	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/cli/exitcode"
	"github.com/KevinLeeNJ/ask/internal/cli/flags"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
	"github.com/KevinLeeNJ/ask/internal/i18n/locale"
	"github.com/KevinLeeNJ/ask/internal/render/pipeline"
	"github.com/KevinLeeNJ/ask/internal/render/terminal"
	configtui "github.com/KevinLeeNJ/ask/internal/tui/config"
	conversationstui "github.com/KevinLeeNJ/ask/internal/tui/conversations"
	"github.com/KevinLeeNJ/ask/internal/version"
)

const (
	defaultSystemZH = "你是一个在终端中回答问题的 AI 助手。优先给出准确、直接、可执行的答案；代码和命令应清晰；不确定时明确说明。"
	defaultSystemEN = "You are an AI assistant answering questions in a terminal. Give accurate, direct, actionable answers; keep code and commands clear; state uncertainty explicitly."
)

type Dependencies struct {
	Configs       ports.ConfigStore
	Secrets       ports.SecretResolver
	Registry      ports.ProviderResolver
	Conversations ports.ConversationRepository
	Leases        ports.ConversationLeaser
	Retention     ports.ConversationRetention
	Routes        ports.ModelRouteStore
	Capabilities  ports.ModelCapabilityResolver
	Shell         ports.ShellProfileWriter
	Clock         ports.Clock
	IDs           ports.IDGenerator
}

type Runner struct {
	deps Dependencies
}

func NewRunner(deps Dependencies) *Runner {
	return &Runner{deps: deps}
}

func (r *Runner) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, parseErr := flags.Parse(args)
	translator := catalog.New(locale.Resolve(options.LanguageOverride, "auto"))
	if parseErr != nil {
		return writeError(stderr, translator, parseErr)
	}

	switch options.Command {
	case flags.CommandHelp:
		_, _ = io.WriteString(stdout, translator.Text("help.text", nil))
		return exitcode.Success
	case flags.CommandVersion:
		_, _ = io.WriteString(stdout, translator.Text("version.text", versionArgs()))
		return exitcode.Success
	case flags.CommandConfig:
		if r.deps.Configs == nil || r.deps.Registry == nil || r.deps.Secrets == nil || r.deps.Shell == nil {
			return writeError(stderr, translator, failure.New(failure.KindInternal, "config.tui_not_available", nil))
		}
		initial := settings.Default()
		if loaded, err := r.deps.Configs.Load(ctx); err == nil {
			initial = loaded
		} else if failure.Info(err).MessageID != "config.not_found" {
			return writeError(stderr, translator, err)
		}
		translator = catalog.New(locale.Resolve(options.LanguageOverride, initial.UI.Language))
		err := configtui.Run(ctx, &configure.Service{
			Configs:  r.deps.Configs,
			Secrets:  r.deps.Secrets,
			Registry: r.deps.Registry,
			Shell:    r.deps.Shell,
			Routes:   r.deps.Routes,
		}, initial, translator)
		if err != nil {
			return writeError(stderr, translator, err)
		}
		return exitcode.Success
	case flags.CommandConversations:
		if r.deps.Configs == nil || r.deps.Conversations == nil || r.deps.IDs == nil || r.deps.Clock == nil {
			return writeError(stderr, translator, failure.New(failure.KindInternal, "conversations.tui_not_available", nil))
		}
		configuredLanguage := "auto"
		if loaded, err := r.deps.Configs.Load(ctx); err == nil {
			configuredLanguage = loaded.UI.Language
		}
		translator = catalog.New(locale.Resolve(options.LanguageOverride, configuredLanguage))
		err := conversationstui.Run(
			ctx,
			r.deps.Conversations,
			r.deps.IDs,
			r.deps.Clock,
			options.ConversationsAction,
			options.ConversationsID,
			translator,
		)
		if err != nil {
			return writeError(stderr, translator, err)
		}
		return exitcode.Success
	}

	stdinCharacters := 0
	if !isTerminal(stdin) {
		input, err := io.ReadAll(stdin)
		if err != nil {
			return writeError(stderr, translator, failure.Wrap(failure.KindConfig, "stdin.read_failed", nil, err))
		}
		stdinText := strings.TrimSuffix(string(input), "\n")
		stdinCharacters = utf8.RuneCountInString(stdinText)
		if strings.TrimSpace(stdinText) != "" {
			if strings.TrimSpace(options.Question) == "" {
				options.Question = stdinText
			} else {
				options.Question = options.Question + "\n\n" + stdinText
			}
		}
	}
	if strings.TrimSpace(options.Question) == "" {
		_, _ = io.WriteString(stderr, translator.Text("usage.no_question", nil)+"\n")
		return exitcode.Usage
	}

	loadedConfig := settings.Default()
	language := locale.Resolve(options.LanguageOverride, "auto")
	if loaded, err := r.deps.Configs.Load(ctx); err == nil {
		loadedConfig = loaded
		language = locale.Resolve(options.LanguageOverride, loaded.UI.Language)
	}
	translator = catalog.New(language)

	terminalColumns, terminalRows := terminalSize(stdout)
	displayCapacity := 0
	if isTerminalWriter(stdout) {
		displayCapacity = ask.EstimateDisplayCapacity(terminalColumns, terminalRows)
	}
	presenter := presenterFor(options, stdout, stderr, language, loadedConfig, terminalColumns)
	service := ask.NewService(ask.Dependencies{
		Configs:       r.deps.Configs,
		Secrets:       r.deps.Secrets,
		Providers:     r.deps.Registry,
		Conversations: r.deps.Conversations,
		Leases:        r.deps.Leases,
		Retention:     r.deps.Retention,
		Routes:        r.deps.Routes,
		Capabilities:  r.deps.Capabilities,
		Clock:         r.deps.Clock,
		IDs:           r.deps.IDs,
	}, presenter)
	result, err := service.Execute(ctx, ask.Options{
		Question:                   options.Question,
		ProviderOverride:           options.ProviderOverride,
		ModelOverride:              options.ModelOverride,
		SystemOverride:             options.SystemOverride,
		ThinkingOverride:           options.ThinkingOverride,
		TimeoutOverride:            options.TimeoutOverride,
		MaxContextMessagesOverride: options.MaxContextMessages,
		ConversationSelector:       options.ConversationID,
		NewConversation:            options.ConversationNew,
		NoStream:                   options.NoStream,
		DefaultSystemZH:            defaultSystemZH,
		DefaultSystemEN:            defaultSystemEN,
		Language:                   language,
		DisplayCapacity:            displayCapacity,
		StdinCharacters:            stdinCharacters,
	})
	if err != nil {
		return writeError(stderr, translator, err)
	}
	if options.JSON {
		if err := writeJSON(stdout, result, options.ShowThinking); err != nil {
			return writeError(stderr, translator, err)
		}
	}
	return exitcode.Success
}

func presenterFor(
	options flags.Options,
	stdout io.Writer,
	stderr io.Writer,
	language string,
	config settings.Config,
	width int,
) ports.Presenter {
	if options.JSON {
		return &silentPresenter{}
	}
	renderMode := options.RenderMode
	if renderMode == "" {
		renderMode = config.UI.RenderMode
	}
	markdownEnabled := config.UI.Markdown != "off"
	if options.Markdown != nil {
		markdownEnabled = *options.Markdown
	}
	colorEnabled := config.UI.Color != "off"
	if options.NoColor || strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		colorEnabled = false
	}
	tty := isTerminalWriter(stdout)
	showThinking := options.ShowThinking
	if options.HideThinking {
		showThinking = false
	} else if !options.ShowThinking {
		showThinking = tty && config.Reasoning.Show != "off"
	}
	presenter, _ := terminal.NewPresenter(
		stdout,
		stderr,
		pipeline.Options{
			RenderMode:   renderMode,
			TTY:          tty,
			Term:         os.Getenv("TERM"),
			Color:        colorEnabled,
			Markdown:     markdownEnabled,
			ShowThinking: showThinking,
			Quiet:        options.Quiet,
			ShowUsage:    true,
			Hyperlinks:   config.UI.Hyperlinks != "off",
			Width:        width,
		},
		language,
	)
	return presenter
}

type jsonResult struct {
	ConversationID string `json:"conversation_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Content        string `json:"content"`
	Status         string `json:"status"`
	Usage          struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	ElapsedMS int64 `json:"elapsed_ms"`
	Reasoning *struct {
		Content         string `json:"content"`
		ReasoningTokens int    `json:"reasoning_tokens,omitempty"`
	} `json:"reasoning,omitempty"`
}

func writeJSON(writer io.Writer, result ask.Result, includeReasoning bool) error {
	output := jsonResult{
		ConversationID: result.ConversationID,
		MessageID:      result.MessageID,
		Provider:       string(result.Response.Provider),
		Model:          result.Response.Model,
		Content:        result.Response.Content,
		Status:         "complete",
		ElapsedMS:      result.Elapsed.Milliseconds(),
	}
	output.Usage.InputTokens = result.Response.Usage.InputTokens
	output.Usage.OutputTokens = result.Response.Usage.OutputTokens
	if includeReasoning && strings.TrimSpace(result.Response.ReasoningContent) != "" {
		output.Reasoning = &struct {
			Content         string `json:"content"`
			ReasoningTokens int    `json:"reasoning_tokens,omitempty"`
		}{
			Content:         result.Response.ReasoningContent,
			ReasoningTokens: result.Response.Usage.ReasoningTokens,
		}
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return failure.Wrap(failure.KindInternal, "output.json_failed", nil, err)
	}
	return nil
}

func writeError(writer io.Writer, translator *catalog.Catalog, err error) int {
	fmt.Fprintf(writer, "ask: %s\n", translator.Error(err))
	return exitcode.FromError(err)
}

func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func terminalSize(writer io.Writer) (int, int) {
	if file, ok := writer.(*os.File); ok {
		if width, height, err := xterm.GetSize(file.Fd()); err == nil && width >= 20 && height >= 5 {
			return width, height
		}
	}
	return terminalDimension("COLUMNS", 80, 20), terminalDimension("LINES", 24, 5)
}

func terminalDimension(name string, fallback, minimum int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	dimension := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return fallback
		}
		dimension = dimension*10 + int(char-'0')
	}
	if dimension < minimum {
		return fallback
	}
	return dimension
}

func versionArgs() map[string]string {
	return map[string]string{
		"version": version.Version,
		"commit":  version.Commit,
		"built":   version.Built,
	}
}

type silentPresenter struct{}

func (*silentPresenter) Progress(stream.Progress)   {}
func (*silentPresenter) ContentDelta(string)        {}
func (*silentPresenter) ReasoningDelta(string)      {}
func (*silentPresenter) Complete(provider.Response) {}
func (*silentPresenter) Fail(error)                 {}

var _ ports.Presenter = (*silentPresenter)(nil)
