package pipeline

import "testing"

func TestResolveModes(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    Mode
	}{
		{name: "auto tty", options: Options{RenderMode: "auto", TTY: true, Term: "xterm-256color"}, want: ModeFull},
		{name: "auto dumb", options: Options{RenderMode: "auto", TTY: true, Term: "dumb"}, want: ModeInline},
		{name: "auto pipe", options: Options{RenderMode: "auto"}, want: ModePlain},
		{name: "forced full fallback", options: Options{RenderMode: "full", TTY: true, Term: "dumb"}, want: ModeInline},
		{name: "inline pipe fallback", options: Options{RenderMode: "inline"}, want: ModePlain},
		{name: "plain", options: Options{RenderMode: "plain", TTY: true, Term: "xterm"}, want: ModePlain},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Resolve(test.options)
			if got.Mode != test.want {
				t.Fatalf("Mode = %q, want %q", got.Mode, test.want)
			}
		})
	}
}

func TestPlainDisablesColorAndMarkdown(t *testing.T) {
	got := Resolve(Options{RenderMode: "plain", Color: true, Markdown: true})
	if got.Color || got.Markdown {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestForcedFullReportsFallback(t *testing.T) {
	got := Resolve(Options{RenderMode: "full", TTY: false, Term: "xterm"})
	if !got.Fallback || got.Mode != ModePlain {
		t.Fatalf("resolved = %#v", got)
	}
}
