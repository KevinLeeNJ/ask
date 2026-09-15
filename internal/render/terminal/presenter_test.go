package terminal

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
	"github.com/KevinLeeNJ/ask/internal/render/pipeline"
)

func TestWaitingAndThinkingUseDistinctSpinnersWithoutStatusText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "full",
			TTY:        true,
			Term:       "xterm",
		},
		"en-US",
	)

	presenter.Progress(stream.Progress{Stage: stream.StageWaiting})
	waitingOutput := stderr.String()
	if strings.Contains(waitingOutput, "Waiting") {
		t.Fatalf("waiting status used text: %q", waitingOutput)
	}
	if !containsAny(waitingOutput, waitingSpinnerFrames) {
		t.Fatalf("waiting spinner frame missing: %q", waitingOutput)
	}

	presenter.Progress(stream.Progress{Stage: stream.StageThinking})
	thinkingOutput := stderr.String()
	if strings.Contains(thinkingOutput, "Thinking") {
		t.Fatalf("thinking status used text: %q", thinkingOutput)
	}
	if !containsAny(thinkingOutput, thinkingSpinnerFrames) {
		t.Fatalf("thinking spinner frame missing: %q", thinkingOutput)
	}
	presenter.Complete(provider.Response{})
}

func TestProgressHintsUseSubtitleStyle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "full",
			TTY:        true,
			Term:       "xterm-256color",
			Color:      true,
			Width:      80,
		},
		"zh-CN",
	)

	presenter.Progress(stream.Progress{Stage: stream.StagePreparing})
	output := stderr.String()
	if !strings.Contains(output, "\x1b[2m\x1b[90m准备配置"+subtleReset) {
		t.Fatalf("progress hint did not use subtitle style: %q", output)
	}
	presenter.Complete(provider.Response{})
}

func TestPlainModePreservesRawMarkdown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "plain",
			Color:      true,
			Markdown:   true,
		},
		"en-US",
	)
	presenter.ContentDelta("# Heading\n\n**bold**\n")
	presenter.Complete(provider.Response{Content: "# Heading\n\n**bold**\n"})
	if stdout.String() != "# Heading\n\n**bold**\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("plain output contains ANSI: %q", stdout.String())
	}
}

func TestFullModeRendersMarkdownSemantically(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "full",
			TTY:        true,
			Term:       "xterm-256color",
			Markdown:   true,
			Width:      40,
		},
		"en-US",
	)
	presenter.ContentDelta("# Heading\n\n**bold**\n")
	response := provider.Response{Content: "# Heading\n\n**bold**\n"}
	presenter.Complete(response)
	plain := stripANSI(stdout.String())
	if strings.Contains(plain, "# Heading") || strings.Contains(plain, "**bold**") {
		t.Fatalf("raw Markdown leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "Heading") || !strings.Contains(plain, "bold") {
		t.Fatalf("semantic content missing:\n%s", plain)
	}
}

func TestReasoningIsSummarizedWhenAnswerStarts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode:   "inline",
			TTY:          true,
			Term:         "xterm",
			Markdown:     false,
			ShowThinking: true,
			ShowUsage:    false,
		},
		"en-US",
	)
	presenter.ReasoningDelta("considering")
	presenter.ContentDelta("answer")
	presenter.Complete(provider.Response{Content: "answer"})
	if stdout.String() != "answer\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	plain := stripANSI(stderr.String())
	if !strings.Contains(plain, "Thinking: considering") ||
		!strings.Contains(plain, "Thinking complete") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMetadataLinesUseSubtleStyleWhenColorEnabled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode:   "full",
			TTY:          true,
			Term:         "xterm-256color",
			Color:        true,
			Markdown:     true,
			ShowThinking: true,
			ShowUsage:    true,
			Width:        80,
		},
		"zh-CN",
	)

	presenter.ReasoningDelta("先分析问题")
	presenter.ContentDelta("回答")
	presenter.Complete(provider.Response{
		Content:  "回答",
		Provider: "opencode",
		Model:    "deepseek-v4.1-flash",
	})

	output := stderr.String()
	plain := stripANSI(output)
	if !strings.Contains(plain, "思考: 先分析问题") {
		t.Fatalf("reasoning text did not use visible content: %q", output)
	}
	if !strings.Contains(output, "\x1b[2m\x1b[90m") {
		t.Fatalf("reasoning text did not use metadata style: %q", output)
	}
	if !strings.Contains(output, "\x1b[2m\x1b[90m思考完成 · ") {
		t.Fatalf("thinking summary was not subdued: %q", output)
	}
	if !strings.Contains(output, "\x1b[2m\x1b[90mprovider=opencode model=deepseek-v4.1-flash") {
		t.Fatalf("usage line was not subdued: %q", output)
	}
	if strings.Count(output, subtleReset) < 3 {
		t.Fatalf("metadata reset count = %d, output = %q", strings.Count(output, subtleReset), output)
	}
}

func TestFullModeReasoningRedrawStaysOnOneTerminalLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode:   "full",
			TTY:          true,
			Term:         "xterm",
			Markdown:     true,
			ShowThinking: true,
			Width:        24,
		},
		"zh-CN",
	)

	presenter.ReasoningDelta("用户问这是一个测试")
	presenter.ReasoningDelta("，助手正在分析较长的思考内容")
	presenter.ReasoningDelta("，并持续刷新同一行")

	presenter.renderMu.Lock()
	if presenter.spinnerCancel == nil || presenter.spinnerKind != spinnerThinking {
		presenter.renderMu.Unlock()
		t.Fatal("reasoning stream did not keep the thinking spinner running")
	}
	parts := strings.Split(stderr.String(), "\r\x1b[2K")
	presenter.renderMu.Unlock()
	if len(parts)-1 < 3 {
		t.Fatalf("reasoning redraw count = %d, stderr = %q", len(parts)-1, stderr.String())
	}
	line := stripANSI(parts[len(parts)-1])
	if strings.Contains(line, "\n") {
		t.Fatalf("reasoning line wrapped: %q", line)
	}
	if width := ansi.StringWidth(line); width > 24 {
		t.Fatalf("reasoning line width = %d, line = %q", width, line)
	}
	if !containsAny(line, thinkingSpinnerFrames) {
		t.Fatalf("reasoning line is missing thinking spinner: %q", line)
	}
	if !strings.Contains(line, " 思考: ") {
		t.Fatalf("reasoning line = %q", line)
	}
	presenter.Complete(provider.Response{})
}

func TestFullModeContentPreviewStaysOnOneTerminalLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "full",
			TTY:        true,
			Term:       "xterm",
			Markdown:   true,
			Width:      24,
		},
		"zh-CN",
	)

	presenter.ContentDelta("这是第一段")
	presenter.ContentDelta("，内容还在持续输出")
	presenter.ContentDelta("，但预览不能折成多行")

	output := stdout.String()
	if strings.Contains(output, "\n") {
		t.Fatalf("content preview emitted a newline: %q", output)
	}
	if parts := strings.Split(output, "\r\x1b[2K"); len(parts) != 4 {
		t.Fatalf("preview redraw count = %d, output = %q", len(parts)-1, output)
	}
	line := output[strings.LastIndex(output, "\r\x1b[2K")+len("\r\x1b[2K"):]
	if width := ansi.StringWidth(line); width > 24 {
		t.Fatalf("preview line width = %d, line = %q", width, line)
	}
}

func TestFullModeCommitsCompletedMarkdownWithoutRedrawingIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter, _ := NewPresenter(
		&stdout,
		&stderr,
		pipeline.Options{
			RenderMode: "full",
			TTY:        true,
			Term:       "xterm",
			Markdown:   true,
			Width:      80,
		},
		"en-US",
	)

	presenter.ContentDelta("first paragraph\n\n")
	presenter.ContentDelta("second paragraph is still streaming")
	presenter.Complete(provider.Response{Content: "first paragraph\n\nsecond paragraph is still streaming"})

	output := stdout.String()
	if count := strings.Count(output, "first paragraph"); count != 1 {
		t.Fatalf("first paragraph count = %d, output = %q", count, output)
	}
	if count := strings.Count(output, "\r\x1b[2K"); count == 0 {
		t.Fatalf("streaming preview was not redrawn: %q", output)
	}
}

func TestLastSafeBoundarySkipsBlankLinesInsideFence(t *testing.T) {
	source := "```go\nfmt.Println(\"first\")\n\nfmt.Println(\"second\")\n```\n\nnext paragraph"
	want := strings.Index(source, "\n\nnext paragraph") + 2
	if got := lastSafeBoundary(source, 0); got != want {
		t.Fatalf("lastSafeBoundary() = %d, want %d", got, want)
	}
}

func stripANSI(input string) string {
	var output strings.Builder
	for index := 0; index < len(input); {
		if input[index] != '\x1b' {
			output.WriteByte(input[index])
			index++
			continue
		}
		index++
		if index >= len(input) {
			break
		}
		if input[index] == '[' {
			index++
			for index < len(input) && (input[index] < '@' || input[index] > '~') {
				index++
			}
			if index < len(input) {
				index++
			}
			continue
		}
		if input[index] == ']' {
			if end := strings.Index(input[index:], "\x1b\\"); end >= 0 {
				index += end + 2
				continue
			}
		}
		index++
	}
	return output.String()
}

func containsAny(input string, values []string) bool {
	for _, value := range values {
		if strings.Contains(input, value) {
			return true
		}
	}
	return false
}
