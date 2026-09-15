package terminal

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
	"github.com/KevinLeeNJ/ask/internal/render/markdown"
	"github.com/KevinLeeNJ/ask/internal/render/pipeline"
)

type Presenter struct {
	stdout     io.Writer
	stderr     io.Writer
	resolved   pipeline.Resolved
	language   string
	started    time.Time
	content    strings.Builder
	reasoning  strings.Builder
	flushed    int
	fullCommit int

	hasContent     bool
	hasReasoning   bool
	progressLine   bool
	contentPreview bool
	reasonStart    time.Time
	renderMu       sync.Mutex

	spinnerCancel chan struct{}
	spinnerKind   spinnerKind
	spinnerFrame  int
	spinnerLine   bool
}

type spinnerKind int

const (
	spinnerWaiting spinnerKind = iota
	spinnerThinking
)

var (
	waitingSpinnerFrames  = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	thinkingSpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
)

const subtleReset = "\x1b[0m"

func NewPresenter(
	stdout io.Writer,
	stderr io.Writer,
	options pipeline.Options,
	language string,
) (*Presenter, pipeline.Resolved) {
	resolved := pipeline.Resolve(options)
	presenter := &Presenter{
		stdout:   stdout,
		stderr:   stderr,
		resolved: resolved,
		language: language,
		started:  time.Now(),
	}
	if resolved.Fallback && !resolved.Quiet {
		fmt.Fprintf(stderr, "%s\n", presenter.subtle(fallbackText(language, resolved.Mode)))
	}
	return presenter, resolved
}

func (p *Presenter) Progress(event stream.Progress) {
	p.renderMu.Lock()
	defer p.renderMu.Unlock()

	if p.resolved.Quiet {
		return
	}
	if p.resolved.Mode != pipeline.ModePlain {
		switch event.Stage {
		case stream.StageWaiting:
			p.startSpinner(spinnerWaiting)
			return
		case stream.StageThinking:
			p.startSpinner(spinnerThinking)
			return
		case stream.StageGenerating:
			p.stopSpinner()
			p.progressLine = false
			return
		}
	}
	p.stopSpinner()
	stage := progressText(p.language, event.Stage, event.ElapsedMs)
	if stage == "" {
		return
	}
	if p.resolved.Mode == pipeline.ModeFull {
		fmt.Fprintf(p.stderr, "\r\x1b[2K%s", p.subtle(stage))
		p.progressLine = true
		return
	}
	fmt.Fprintln(p.stderr, p.subtle(stage))
}

func (p *Presenter) ContentDelta(text string) {
	p.renderMu.Lock()
	defer p.renderMu.Unlock()

	if text == "" {
		return
	}
	if !p.hasContent {
		p.stopSpinner()
		p.finishReasoning()
		p.finishProgressLine()
		p.hasContent = true
	}
	p.content.WriteString(text)

	switch {
	case p.resolved.Mode == pipeline.ModePlain:
		_, _ = io.WriteString(p.stdout, text)
	case !p.resolved.Markdown:
		_, _ = io.WriteString(p.stdout, text)
	case p.resolved.Mode == pipeline.ModeInline:
		p.flushInline(false)
	default:
		p.streamFull()
	}
}

func (p *Presenter) ReasoningDelta(text string) {
	p.renderMu.Lock()
	defer p.renderMu.Unlock()

	if !p.resolved.ShowThinking || p.resolved.Quiet || text == "" {
		return
	}
	first := !p.hasReasoning
	if first {
		p.finishProgressLine()
		p.reasonStart = time.Now()
		p.hasReasoning = true
	}
	p.reasoning.WriteString(text)
	if p.resolved.Mode == pipeline.ModeFull {
		if p.spinnerCancel == nil || p.spinnerKind != spinnerThinking {
			p.startSpinner(spinnerThinking)
		} else {
			p.drawSpinnerFrame(spinnerThinking)
		}
		return
	}
	p.stopSpinner()
	if first {
		p.reasoningTextPrefix()
	}
	_, _ = io.WriteString(p.stderr, p.subtle(text))
}

func (p *Presenter) Complete(response provider.Response) {
	p.renderMu.Lock()
	defer p.renderMu.Unlock()

	p.stopSpinner()
	p.finishReasoning()
	p.finishProgressLine()
	if p.hasContent {
		switch {
		case p.resolved.Mode == pipeline.ModeFull && p.resolved.Markdown:
			p.finishFull()
		case p.resolved.Mode == pipeline.ModeInline && p.resolved.Markdown:
			p.flushInline(true)
		case !strings.HasSuffix(response.Content, "\n"):
			_, _ = io.WriteString(p.stdout, "\n")
		}
	}
	if p.resolved.Quiet || !p.resolved.ShowUsage {
		return
	}
	elapsed := time.Since(p.started)
	fmt.Fprintf(
		p.stderr,
		"%s\n",
		p.subtle(fmt.Sprintf(
			"provider=%s model=%s elapsed=%s input=%d output=%d",
			response.Provider,
			response.Model,
			elapsed.Round(time.Millisecond),
			response.Usage.InputTokens,
			response.Usage.OutputTokens,
		)),
	)
}

func (p *Presenter) subtle(text string) string {
	if !p.resolved.Color {
		return text
	}
	return "\x1b[2m\x1b[90m" + text + subtleReset
}

func (p *Presenter) Fail(error) {
	p.renderMu.Lock()
	defer p.renderMu.Unlock()

	p.stopSpinner()
	p.finishProgressLine()
	if p.hasContent && p.resolved.Mode == pipeline.ModeFull && p.resolved.Markdown {
		p.finishFull()
	} else if p.hasContent && !strings.HasSuffix(p.content.String(), "\n") {
		_, _ = io.WriteString(p.stdout, "\n")
	}
}

func (p *Presenter) finishProgressLine() {
	if !p.progressLine {
		return
	}
	if p.resolved.Mode == pipeline.ModeFull {
		fmt.Fprint(p.stderr, "\r\x1b[2K")
	} else {
		fmt.Fprintln(p.stderr)
	}
	p.progressLine = false
}

func (p *Presenter) startSpinner(kind spinnerKind) {
	if p.spinnerCancel != nil && p.spinnerKind == kind {
		return
	}
	p.stopSpinner()
	p.finishProgressLine()

	cancel := make(chan struct{})
	p.spinnerCancel = cancel
	p.spinnerKind = kind
	p.spinnerFrame = 0
	p.drawSpinnerFrame(kind)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-cancel:
				return
			case <-ticker.C:
				p.renderMu.Lock()
				if p.spinnerCancel != cancel {
					p.renderMu.Unlock()
					return
				}
				p.spinnerFrame++
				p.drawSpinnerFrame(kind)
				p.renderMu.Unlock()
			}
		}
	}()
}

func (p *Presenter) stopSpinner() {
	if p.spinnerCancel == nil {
		return
	}
	close(p.spinnerCancel)
	p.spinnerCancel = nil
	p.spinnerKind = 0
	p.spinnerFrame = 0
	if p.spinnerLine {
		fmt.Fprint(p.stderr, "\r\x1b[2K")
		p.spinnerLine = false
	}
	p.progressLine = false
}

func (p *Presenter) drawSpinnerFrame(kind spinnerKind) {
	frames := waitingSpinnerFrames
	if kind == spinnerThinking {
		frames = thinkingSpinnerFrames
	}
	frame := frames[p.spinnerFrame%len(frames)]
	line := frame + " "
	if kind == spinnerThinking && p.hasReasoning && p.resolved.ShowThinking {
		line += reasoningLine(
			p.language,
			p.reasoning.String(),
			p.resolved.Width-ansi.StringWidth(line),
		)
	}
	fmt.Fprintf(p.stderr, "\r\x1b[2K%s", p.subtle(line))
	p.spinnerLine = true
	p.progressLine = true
}

func (p *Presenter) reasoningTextPrefix() {
	if p.resolved.Mode == pipeline.ModeFull {
		fmt.Fprintf(p.stderr, "%s", p.subtle(reasoningLabel(p.language)+" "))
		return
	}
	_, _ = io.WriteString(p.stderr, p.subtle(reasoningLabel(p.language)+" "))
}

func (p *Presenter) finishReasoning() {
	if p.hasReasoning {
		elapsed := time.Since(p.reasonStart).Round(100 * time.Millisecond)
		summary := p.subtle(fmt.Sprintf(thinkingSummary(p.language), elapsed))
		if p.resolved.Mode == pipeline.ModeFull {
			fmt.Fprintf(p.stderr, "\r\x1b[2K%s\n", summary)
			p.progressLine = false
		} else {
			if !strings.HasSuffix(p.reasoning.String(), "\n") {
				_, _ = io.WriteString(p.stderr, "\n")
			}
			fmt.Fprintln(p.stderr, summary)
		}
		p.hasReasoning = false
	}
	p.progressLine = false
}

func (p *Presenter) streamFull() {
	content := p.content.String()
	if boundary := lastSafeBoundary(content, p.fullCommit); boundary > p.fullCommit {
		p.clearContentPreview()
		_, _ = io.WriteString(p.stdout, p.renderMarkdown(content[p.fullCommit:boundary]))
		p.fullCommit = boundary
	}
	p.renderContentPreview(content[p.fullCommit:])
}

func (p *Presenter) finishFull() {
	p.clearContentPreview()
	content := p.content.String()
	if p.fullCommit >= len(content) {
		return
	}
	_, _ = io.WriteString(p.stdout, p.renderMarkdown(content[p.fullCommit:]))
	p.fullCommit = len(content)
}

func (p *Presenter) renderContentPreview(input string) {
	if input == "" {
		p.clearContentPreview()
		return
	}
	preview := collapseWhitespace(p.renderMarkdown(input))
	if preview == "" {
		p.clearContentPreview()
		return
	}
	line := ansi.Truncate(tail(preview, p.resolved.Width), p.resolved.Width, "")
	fmt.Fprintf(p.stdout, "\r\x1b[2K%s", line)
	p.contentPreview = true
}

func (p *Presenter) clearContentPreview() {
	if !p.contentPreview {
		return
	}
	fmt.Fprint(p.stdout, "\r\x1b[2K")
	p.contentPreview = false
}

func (p *Presenter) flushInline(force bool) {
	content := p.content.String()
	if p.flushed >= len(content) {
		return
	}
	pending := content[p.flushed:]
	cut := strings.LastIndexByte(pending, '\n')
	if cut < 0 && !force {
		return
	}
	if cut < 0 {
		cut = len(pending) - 1
	}
	chunk := pending[:cut+1]
	rendered := p.renderMarkdown(chunk)
	_, _ = io.WriteString(p.stdout, rendered)
	p.flushed += cut + 1
}

func (p *Presenter) renderMarkdown(input string) string {
	return markdown.Render(
		[]byte(input),
		markdown.Options{
			Color:      p.resolved.Color,
			Hyperlinks: p.resolved.Hyperlinks,
			Width:      p.resolved.Width,
		},
	)
}

func progressText(language string, stage stream.ProgressStage, elapsedMs int64) string {
	zh := language == "zh-CN"
	elapsed := ""
	if elapsedMs >= 1000 {
		elapsed = fmt.Sprintf(" · %.1fs", float64(elapsedMs)/1000)
	}
	switch stage {
	case stream.StagePreparing:
		if zh {
			return "准备配置" + elapsed
		}
		return "Preparing configuration" + elapsed
	case stream.StageConnecting:
		if zh {
			return "连接服务" + elapsed
		}
		return "Connecting" + elapsed
	case stream.StageWaiting:
		if zh {
			return "等待首个事件" + elapsed
		}
		return "Waiting for the first event" + elapsed
	case stream.StageThinking:
		if zh {
			return "思考中" + elapsed
		}
		return "Thinking" + elapsed
	case stream.StageGenerating:
		if zh {
			return "生成中" + elapsed
		}
		return "Generating" + elapsed
	case stream.StageComplete:
		if zh {
			return "已完成"
		}
		return "Completed"
	default:
		return ""
	}
}

func fallbackText(language string, mode pipeline.Mode) string {
	if language == "zh-CN" {
		return fmt.Sprintf("终端能力不足，已降级为 %s 渲染模式", mode)
	}
	return fmt.Sprintf("terminal capabilities are limited; falling back to %s rendering", mode)
}

func reasoningLabel(language string) string {
	if language == "zh-CN" {
		return "思考:"
	}
	return "Thinking:"
}

func thinkingSummary(language string) string {
	if language == "zh-CN" {
		return "思考完成 · %s"
	}
	return "Thinking complete · %s"
}

func collapseWhitespace(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

func lastSafeBoundary(input string, start int) int {
	boundary := start
	var fenceChar byte
	fenceLength := 0
	for lineStart := start; lineStart < len(input); {
		offset := strings.IndexByte(input[lineStart:], '\n')
		if offset < 0 {
			break
		}
		lineEnd := lineStart + offset
		line := input[lineStart:lineEnd]
		trimmed := strings.TrimSpace(line)
		if fenceChar != 0 {
			if isClosingFence(trimmed, fenceChar, fenceLength) {
				fenceChar = 0
				fenceLength = 0
				boundary = lineEnd + 1
			}
		} else if char, length, ok := openingFence(line); ok {
			fenceChar = char
			fenceLength = length
		} else if trimmed == "" {
			boundary = lineEnd + 1
		}
		lineStart = lineEnd + 1
	}
	return boundary
}

func openingFence(line string) (byte, int, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}
	if length < 3 {
		return 0, 0, false
	}
	if char == '`' && strings.ContainsRune(trimmed[length:], '`') {
		return 0, 0, false
	}
	return char, length, true
}

func isClosingFence(line string, char byte, minimum int) bool {
	length := 0
	for length < len(line) && line[length] == char {
		length++
	}
	return length >= minimum && strings.TrimSpace(line[length:]) == ""
}

func reasoningLine(language, text string, width int) string {
	prefix := reasoningLabel(language) + " "
	remaining := width - ansi.StringWidth(prefix)
	if remaining < 1 {
		return ansi.Truncate(prefix, width, "")
	}
	line := prefix + tail(collapseWhitespace(text), remaining)
	return ansi.Truncate(line, width, "")
}

func tail(input string, width int) string {
	if width <= 0 {
		return ""
	}
	inputWidth := ansi.StringWidth(input)
	if inputWidth <= width {
		return input
	}
	const marker = "…"
	markerWidth := ansi.StringWidth(marker)
	if width <= markerWidth {
		return ansi.Truncate(marker, width, "")
	}
	start := inputWidth - (width - markerWidth)
	for start < inputWidth {
		candidate := marker + ansi.Cut(input, start, inputWidth)
		if excess := ansi.StringWidth(candidate) - width; excess > 0 {
			start += excess
			continue
		}
		return candidate
	}
	return ansi.Truncate(marker, width, "")
}
