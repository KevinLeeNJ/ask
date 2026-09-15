package pipeline

import "strings"

type Mode string

const (
	ModeFull   Mode = "full"
	ModeInline Mode = "inline"
	ModePlain  Mode = "plain"
)

type Options struct {
	RenderMode   string
	TTY          bool
	Term         string
	Color        bool
	Markdown     bool
	ShowThinking bool
	Quiet        bool
	ShowUsage    bool
	Hyperlinks   bool
	Width        int
}

type Resolved struct {
	Mode         Mode
	Color        bool
	Markdown     bool
	ShowThinking bool
	Quiet        bool
	ShowUsage    bool
	Hyperlinks   bool
	Width        int
	Fallback     bool
}

func Resolve(options Options) Resolved {
	mode := strings.ToLower(strings.TrimSpace(options.RenderMode))
	if mode == "" {
		mode = "auto"
	}
	safeCursor := options.TTY && !strings.EqualFold(strings.TrimSpace(options.Term), "dumb")
	requestedFull := mode == string(ModeFull)
	switch mode {
	case string(ModeFull):
		if !safeCursor {
			if options.TTY {
				mode = string(ModeInline)
			} else {
				mode = string(ModePlain)
			}
		}
	case string(ModeInline):
		if !options.TTY {
			mode = string(ModePlain)
		}
	case string(ModePlain):
	default:
		if safeCursor {
			mode = string(ModeFull)
		} else if options.TTY {
			mode = string(ModeInline)
		} else {
			mode = string(ModePlain)
		}
	}
	if options.Width <= 0 {
		options.Width = 80
	}
	resolvedMode := Mode(mode)
	return Resolved{
		Mode:         resolvedMode,
		Color:        options.Color && resolvedMode != ModePlain,
		Markdown:     options.Markdown && resolvedMode != ModePlain,
		ShowThinking: options.ShowThinking,
		Quiet:        options.Quiet,
		ShowUsage:    options.ShowUsage,
		Hyperlinks:   options.Hyperlinks,
		Width:        options.Width,
		Fallback:     requestedFull && resolvedMode != ModeFull,
	}
}
