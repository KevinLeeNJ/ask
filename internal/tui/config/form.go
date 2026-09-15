package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	xterm "github.com/charmbracelet/x/term"

	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
)

var (
	errCancelled = errors.New("form cancelled")
	errExitApp   = errors.New("application exit requested")
)

func runForm(ctx context.Context, translator *catalog.Catalog, fields ...huh.Field) error {
	form := huh.NewForm(huh.NewGroup(fields...)).
		WithTheme(huh.ThemeCharm()).
		WithAccessible(false).
		WithKeyMap(keyMap(translator))
	return runInteractiveForm(ctx, form)
}

func runBuiltForm(ctx context.Context, translator *catalog.Catalog, form *huh.Form) error {
	return runInteractiveForm(ctx, form.
		WithAccessible(false).
		WithKeyMap(keyMap(translator)))
}

func runModelForm(ctx context.Context, translator *catalog.Catalog, form *huh.Form) error {
	return runInteractiveForm(ctx, form.
		WithAccessible(false).
		WithKeyMap(modelKeyMap(translator)))
}

func runRootForm(ctx context.Context, translator *catalog.Catalog, form *huh.Form) error {
	return runInteractiveForm(ctx, form.
		WithAccessible(false).
		WithKeyMap(rootKeyMap(translator)))
}

func runSystemPromptForm(
	ctx context.Context,
	translator *catalog.Catalog,
	value *string,
) error {
	form := huh.NewForm(huh.NewGroup(
		huh.NewText().
			Title(translator.Text("menu.system_prompt.title", nil)).
			Value(value),
	)).WithTheme(huh.ThemeCharm())
	return runInteractiveForm(ctx, form.
		WithAccessible(false).
		WithKeyMap(systemPromptKeyMap(translator)))
}

type globalQuitForm struct {
	form          *huh.Form
	exitRequested bool
}

func (model *globalQuitForm) Init() tea.Cmd {
	return model.form.Init()
}

func (model *globalQuitForm) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMessage, ok := message.(tea.KeyMsg); ok && keyMessage.Type == tea.KeyCtrlC {
		model.exitRequested = true
		return model, tea.Quit
	}
	updated, command := model.form.Update(message)
	if form, ok := updated.(*huh.Form); ok {
		model.form = form
	}
	return model, command
}

func (model *globalQuitForm) View() string {
	return model.form.View()
}

type providerSelectForm struct {
	form            *huh.Form
	field           *huh.Select[string]
	deleteRequested bool
	deleteValue     string
	cancelRequested bool
	exitRequested   bool
	filtering       bool
}

func (model *providerSelectForm) Init() tea.Cmd {
	return model.form.Init()
}

func (model *providerSelectForm) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMessage, ok := message.(tea.KeyMsg); ok && keyMessage.Type == tea.KeyCtrlC {
		model.exitRequested = true
		return model, tea.Quit
	}

	if keyMessage, ok := message.(tea.KeyMsg); ok {
		switch keyMessage.Type {
		case tea.KeyEsc:
			if !model.filtering {
				model.cancelRequested = true
				return model, tea.Quit
			}
			model.filtering = false
		case tea.KeyEnter:
			model.filtering = false
		case tea.KeyRunes:
			if !model.filtering && keyMessage.String() == "/" {
				model.filtering = true
			}
		}
		if !model.filtering && (keyMessage.Type == tea.KeyDelete || keyMessage.Type == tea.KeyBackspace) {
			if value, ok := model.field.Hovered(); ok && !isReservedProviderSelection(value) {
				model.deleteRequested = true
				model.deleteValue = value
				return model, tea.Quit
			}
		}
	}

	updated, command := model.form.Update(message)
	if form, ok := updated.(*huh.Form); ok {
		model.form = form
	}
	return model, command
}

func (model *providerSelectForm) View() string {
	return model.form.View()
}

func runProviderSelection(
	ctx context.Context,
	field *huh.Select[string],
	translator *catalog.Catalog,
) (string, bool, error) {
	if os.Getenv("TERM") == "dumb" {
		return "", false, runForm(ctx, translator, field)
	}
	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(huh.ThemeCharm()).
		WithKeyMap(providerSelectKeyMap(translator))
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Interrupt

	model, err := tea.NewProgram(
		&providerSelectForm{form: form, field: field},
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithReportFocus(),
	).Run()
	clearRenderedView(os.Stderr)
	result, ok := model.(*providerSelectForm)
	if ok && result.deleteRequested {
		return result.deleteValue, true, nil
	}
	if ok && result.cancelRequested {
		return "", false, errCancelled
	}
	if ok && result.exitRequested {
		return "", false, errExitApp
	}
	if ok && result.form.State == huh.StateAborted {
		return "", false, errCancelled
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return "", false, errCancelled
	}
	if errors.Is(err, tea.ErrProgramKilled) {
		return "", false, huh.ErrTimeout
	}
	if err != nil {
		return "", false, fmt.Errorf("huh: %w", err)
	}
	return "", false, nil
}

func providerSelectKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	value := huh.NewDefaultKeyMap()
	compact := terminalWidth() < 60
	localizeKeyMap(value, translator, compact)
	value.Quit = key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", translator.Text("menu.help.back", nil)),
	)
	if compact {
		value.Select.Down.SetHelp("del", translator.Text("menu.help.compact_delete_down", nil))
	} else {
		value.Select.Down.SetHelp("↓", translator.Text("menu.help.delete_down", nil))
	}
	value.Select.Next.SetHelp("enter", translator.Text("menu.help.edit_exit", nil))
	value.Select.Submit.SetHelp("enter", translator.Text("menu.help.edit_exit", nil))
	return value
}

func isReservedProviderSelection(value string) bool {
	return value == providerActionAdd || value == providerActionBack
}

func runInteractiveForm(ctx context.Context, form *huh.Form) error {
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Interrupt
	if height := fittedFormHeight(terminalHeight()); height > 0 {
		form.WithHeight(height)
	}
	model, err := tea.NewProgram(
		&globalQuitForm{form: form},
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithReportFocus(),
	).Run()
	clearRenderedView(os.Stderr)
	result, ok := model.(*globalQuitForm)
	if ok && result.exitRequested {
		return errExitApp
	}
	if ok && result.form.State == huh.StateAborted {
		return errCancelled
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return errCancelled
	}
	if errors.Is(err, tea.ErrProgramKilled) {
		return huh.ErrTimeout
	}
	if err != nil {
		return fmt.Errorf("huh: %w", err)
	}
	return nil
}

func clearRenderedView(writer io.Writer) {
	_, _ = io.WriteString(writer, "\x1b[2J\x1b[H")
}

func terminalHeight() int {
	if _, height, err := xterm.GetSize(os.Stderr.Fd()); err == nil {
		return height
	}
	return 0
}

func terminalWidth() int {
	if width, _, err := xterm.GetSize(os.Stderr.Fd()); err == nil {
		return width
	}
	return 80
}

func fittedFormHeight(terminalHeight int) int {
	if terminalHeight <= 0 {
		return 0
	}
	if terminalHeight <= 4 {
		return terminalHeight
	}
	return terminalHeight - 1
}

func keyMap(translator *catalog.Catalog) *huh.KeyMap {
	nextHelp := translator.Text("menu.help.next_back_quit", nil)
	if terminalWidth() < 60 {
		nextHelp = translator.Text("menu.help.next", nil)
	}
	return formKeyMap(
		nextHelp,
		translator.Text("menu.help.back", nil),
		translator,
		terminalWidth() < 60,
	)
}

func modelKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	compact := terminalWidth() < 60
	confirm := translator.Text("menu.help.confirm_back_quit", nil)
	if compact {
		confirm = translator.Text("menu.help.confirm", nil)
	}
	value := formKeyMap(confirm, translator.Text("menu.help.back", nil), translator, compact)
	value.MultiSelect.Toggle.SetHelp("space/x", translator.Text("menu.help.toggle", nil))
	value.MultiSelect.Filter.SetHelp("/", translator.Text("menu.help.search", nil))
	value.MultiSelect.Submit.SetHelp("enter", confirm)
	value.MultiSelect.Next.SetHelp("enter", confirm)
	return value
}

func systemPromptKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	value := keyMap(translator)
	value.Text.Submit = key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", translator.Text("menu.action.save", nil)),
	)
	return value
}

func rootKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	nextHelp := translator.Text("menu.help.select_exit_quit", nil)
	quitHelp := translator.Text("menu.help.exit", nil)
	if terminalWidth() < 60 {
		nextHelp = translator.Text("menu.help.select", nil)
	}
	return formKeyMap(
		nextHelp,
		quitHelp,
		translator,
		terminalWidth() < 60,
	)
}

func formKeyMap(
	nextHelp string,
	quitHelp string,
	translator *catalog.Catalog,
	compact bool,
) *huh.KeyMap {
	value := huh.NewDefaultKeyMap()
	localizeKeyMap(value, translator, compact)
	value.Quit = key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", quitHelp),
	)
	value.Input.Next.SetHelp("enter", nextHelp)
	value.Input.Submit.SetHelp("enter", nextHelp)
	value.Text.Next.SetHelp("enter", nextHelp)
	value.Text.Submit.SetHelp("enter", nextHelp)
	value.Select.Next.SetHelp("enter", nextHelp)
	value.Select.Submit.SetHelp("enter", nextHelp)
	value.MultiSelect.Next.SetHelp("enter", nextHelp)
	value.MultiSelect.Submit.SetHelp("enter", nextHelp)
	value.Confirm.Next.SetHelp("enter", nextHelp)
	value.Confirm.Submit.SetHelp("enter", nextHelp)
	value.Note.Next.SetHelp("enter", nextHelp)
	value.Note.Submit.SetHelp("enter", nextHelp)
	return value
}

func localizeKeyMap(value *huh.KeyMap, translator *catalog.Catalog, compact bool) {
	move := translator.Text("menu.help.move", nil)
	search := translator.Text("menu.help.search", nil)

	value.Input.AcceptSuggestion.SetHelp("ctrl+e", translator.Text("menu.help.complete", nil))
	value.Text.NewLine.SetHelp("alt+enter / ctrl+j", translator.Text("menu.help.new_line", nil))
	value.Text.Editor.SetHelp("ctrl+e", translator.Text("menu.help.editor", nil))

	value.Select.HalfPageUp.SetHelp("ctrl+u", translator.Text("menu.help.page_up", nil))
	value.Select.HalfPageDown.SetHelp("ctrl+d", translator.Text("menu.help.page_down", nil))
	value.Select.GotoTop.SetHelp("g/home", translator.Text("menu.help.first", nil))
	value.Select.GotoBottom.SetHelp("G/end", translator.Text("menu.help.last", nil))

	value.MultiSelect.HalfPageUp.SetHelp("ctrl+u", translator.Text("menu.help.page_up", nil))
	value.MultiSelect.HalfPageDown.SetHelp("ctrl+d", translator.Text("menu.help.page_down", nil))
	value.MultiSelect.GotoTop.SetHelp("g/home", translator.Text("menu.help.first", nil))
	value.MultiSelect.GotoBottom.SetHelp("G/end", translator.Text("menu.help.last", nil))
	value.MultiSelect.SelectAll.SetHelp("ctrl+a", translator.Text("menu.help.select_all", nil))
	value.MultiSelect.SelectNone.SetHelp("ctrl+a", translator.Text("menu.help.select_none", nil))

	value.Confirm.Toggle.SetHelp("←/→", translator.Text("menu.help.toggle", nil))

	value.Select.Up.SetHelp("↑", move)
	value.Select.Down.SetHelp("↓", move)
	value.Select.Filter.SetHelp("/", search)
	value.MultiSelect.Up.SetHelp("↑/k", move)
	value.MultiSelect.Down.SetHelp("↓/j", move)
	value.MultiSelect.Filter.SetHelp("/", search)
}

type loadingDoneMsg struct {
	err error
}

type loadingModel struct {
	spinner     spinner.Model
	message     string
	cancel      context.CancelFunc
	work        func() error
	err         error
	interrupted bool
}

func (model *loadingModel) Init() tea.Cmd {
	return tea.Batch(
		model.spinner.Tick,
		func() tea.Msg {
			return loadingDoneMsg{err: model.work()}
		},
	)
}

func (model *loadingModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(message)
		return model, command
	case loadingDoneMsg:
		model.err = message.err
		return model, tea.Quit
	case tea.KeyMsg:
		if message.Type == tea.KeyCtrlC {
			model.interrupted = true
			if model.cancel != nil {
				model.cancel()
			}
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *loadingModel) View() string {
	return model.spinner.View() + " " + model.message
}

func runLoading(
	ctx context.Context,
	message string,
	work func(context.Context) error,
) error {
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintln(os.Stderr, message)
		return work(ctx)
	}

	workContext, cancel := context.WithCancel(ctx)
	defer cancel()
	model := &loadingModel{
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		message: message,
		cancel:  cancel,
		work: func() error {
			return work(workContext)
		},
	}
	result, err := tea.NewProgram(
		model,
		tea.WithContext(workContext),
		tea.WithOutput(os.Stderr),
		tea.WithReportFocus(),
	).Run()
	_, _ = io.WriteString(os.Stderr, "\r\x1b[2K")
	finished, ok := result.(*loadingModel)
	if ok && finished.interrupted {
		return errExitApp
	}
	if ok && finished.err != nil {
		return finished.err
	}
	if errors.Is(err, tea.ErrInterrupted) ||
		errors.Is(err, tea.ErrProgramKilled) ||
		errors.Is(err, context.Canceled) {
		return errExitApp
	}
	return err
}
