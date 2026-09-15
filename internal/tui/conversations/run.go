package conversations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	xterm "github.com/charmbracelet/x/term"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	titlegen "github.com/KevinLeeNJ/ask/internal/conversation/title"
	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
	"github.com/KevinLeeNJ/ask/internal/tui/widgets"
)

const (
	actionNew  = "__new__"
	actionBack = "__back__"
)

var conversationColumnWidths = []int{32, 12, 8}

func Run(
	ctx context.Context,
	repository ports.ConversationRepository,
	ids ports.IDGenerator,
	now ports.Clock,
	action string,
	selector string,
	translator *catalog.Catalog,
) error {
	switch strings.ToLower(action) {
	case "list":
		values, err := repository.List(ctx, 200)
		if err != nil {
			return err
		}
		printList(values, translator)
		return nil
	case "delete":
		return deleteConversation(ctx, repository, selector, translator)
	case "rename":
		return renameConversation(ctx, repository, selector, translator)
	default:
		return selectConversation(ctx, repository, ids, now, translator)
	}
}

func selectConversation(
	ctx context.Context,
	repository ports.ConversationRepository,
	ids ports.IDGenerator,
	now ports.Clock,
	translator *catalog.Catalog,
) error {
	selected := ""
	for {
		values, err := repository.List(ctx, 200)
		if err != nil {
			return err
		}
		if !conversationSelectionExists(values, selected) {
			selected = initialConversationSelection(values)
		}
		field := huh.NewSelect[string]().
			Title(translator.Text("menu.conversations.select", nil)).
			Description(conversationColumns(translator, terminalWidth())).
			Options(selectionOptions(values, translator, terminalWidth())...).
			Value(&selected)
		deleteValue, deleteRequested, err := runConversationSelection(ctx, field, translator)
		if err != nil {
			return err
		}
		if deleteRequested {
			if err := deleteConversation(ctx, repository, deleteValue, translator); err != nil {
				return err
			}
			continue
		}
		switch selected {
		case actionBack:
			return nil
		case actionNew:
			idValue, err := ids.New()
			if err != nil {
				return err
			}
			timestamp := now.Now()
			value := conversation.Conversation{
				ID:          conversation.ID(idValue),
				Title:       titlegen.Provisional(translator.Text("menu.conversations.new", nil)),
				TitleSource: conversation.TitleProvisional,
				CreatedAt:   timestamp,
				UpdatedAt:   timestamp,
			}
			if err := repository.Create(ctx, value); err != nil {
				return err
			}
			if err := repository.SetCurrent(ctx, value.ID); err != nil {
				return err
			}
			return nil
		default:
			if err := repository.SetCurrent(ctx, conversation.ID(selected)); err != nil {
				return err
			}
			return nil
		}
	}
}

func initialConversationSelection(values []conversation.Conversation) string {
	if len(values) > 0 {
		return string(values[0].ID)
	}
	return actionNew
}

func conversationSelectionExists(values []conversation.Conversation, selected string) bool {
	switch selected {
	case actionNew, actionBack:
		return true
	case "":
		return false
	}
	for _, value := range values {
		if string(value.ID) == selected {
			return true
		}
	}
	return false
}

func deleteConversation(
	ctx context.Context,
	repository ports.ConversationRepository,
	selector string,
	translator *catalog.Catalog,
) error {
	value, err := resolve(ctx, repository, selector, translator)
	if err != nil || value.ID == "" {
		return err
	}
	confirmed := false
	if err := runForm(ctx, translator, huh.NewConfirm().
		Title(translator.Text("menu.conversations.delete_title", nil)).
		Description(value.Title).
		Affirmative(translator.Text("menu.action.yes", nil)).
		Negative(translator.Text("menu.action.no", nil)).
		Value(&confirmed)); err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	return repository.Delete(ctx, value.ID)
}

func renameConversation(
	ctx context.Context,
	repository ports.ConversationRepository,
	selector string,
	translator *catalog.Catalog,
) error {
	value, err := resolve(ctx, repository, selector, translator)
	if err != nil || value.ID == "" {
		return err
	}
	title := value.Title
	if err := runForm(ctx, translator, huh.NewInput().
		Title(translator.Text("menu.conversations.new_title", nil)).
		Value(&title)); err != nil {
		return err
	}
	return repository.Rename(ctx, value.ID, title)
}

func resolve(
	ctx context.Context,
	repository ports.ConversationRepository,
	selector string,
	translator *catalog.Catalog,
) (conversation.Conversation, error) {
	if strings.TrimSpace(selector) == "" {
		values, err := repository.List(ctx, 200)
		if err != nil {
			return conversation.Conversation{}, err
		}
		if len(values) == 0 {
			return conversation.Conversation{}, nil
		}
		selected := ""
		options := make([]huh.Option[string], 0, len(values))
		for _, value := range values {
			options = append(
				options,
				huh.NewOption(formatConversation(value, translator, terminalWidth()), string(value.ID)),
			)
		}
		if err := runForm(ctx, translator, huh.NewSelect[string]().
			Title(translator.Text("menu.conversations.select", nil)).
			Description(conversationColumns(translator, terminalWidth())).
			Options(options...).
			Value(&selected)); err != nil {
			return conversation.Conversation{}, err
		}
		if selected == "" {
			return conversation.Conversation{}, nil
		}
		return repository.Get(ctx, conversation.ID(selected))
	}
	return repository.FindByPrefix(ctx, selector)
}

func printList(values []conversation.Conversation, translator *catalog.Catalog) {
	if len(values) == 0 {
		fmt.Fprintln(os.Stdout, translator.Text("menu.conversations.empty", nil))
		return
	}
	for _, value := range values {
		fmt.Fprintf(
			os.Stdout,
			"%s  %s  %d  %s/%s\n",
			value.ID,
			value.Title,
			value.MessageCount,
			value.Provider,
			value.Model,
		)
	}
}

func formatConversation(
	value conversation.Conversation,
	translator *catalog.Catalog,
	width int,
) string {
	updated := relativeTime(value.UpdatedAt, translator)
	title := value.Title
	if strings.TrimSpace(title) == "" {
		title = translator.Text("menu.conversations.untitled", nil)
	}
	if width < 60 {
		return widgets.FormatCompact(
			[]string{title, updated, fmt.Sprintf("%d", value.MessageCount)},
			max(width-2, 1),
		)
	}
	return widgets.FormatColumns(
		[]string{title, updated, fmt.Sprintf("%d", value.MessageCount)},
		conversationColumnWidths,
	)
}

func conversationColumns(translator *catalog.Catalog, width int) string {
	labels := []string{
		translator.Text("menu.conversations.columns.title", nil),
		translator.Text("menu.conversations.columns.updated", nil),
		translator.Text("menu.conversations.columns.messages", nil),
	}
	if width < 60 {
		return "  " + widgets.CompactHeader(labels, max(width-2, 1))
	}
	return "  " + widgets.FormatColumns(
		labels,
		conversationColumnWidths,
	)
}

func selectionOptions(
	values []conversation.Conversation,
	translator *catalog.Catalog,
	width int,
) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(values)+2)
	for _, value := range values {
		options = append(options, huh.NewOption(formatConversation(value, translator, width), string(value.ID)))
	}
	options = append(
		options,
		huh.NewOption(translator.Text("menu.action.new_conversation", nil), actionNew),
		huh.NewOption(translator.Text("menu.action.back", nil), actionBack),
	)
	return options
}

func relativeTime(value time.Time, translator *catalog.Catalog) string {
	elapsed := time.Since(value)
	switch {
	case elapsed < time.Minute:
		return translator.Text("menu.conversations.relative.now", nil)
	case elapsed < time.Hour:
		return translator.Text("menu.conversations.relative.minutes", map[string]string{
			"count": fmt.Sprintf("%d", int(elapsed.Minutes())),
		})
	case elapsed < 24*time.Hour:
		return translator.Text("menu.conversations.relative.hours", map[string]string{
			"count": fmt.Sprintf("%d", int(elapsed.Hours())),
		})
	default:
		return translator.Text("menu.conversations.relative.days", map[string]string{
			"count": fmt.Sprintf("%d", int(elapsed.Hours()/24)),
		})
	}
}

func runForm(ctx context.Context, translator *catalog.Catalog, fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).
		WithTheme(huh.ThemeCharm()).
		WithAccessible(false).
		WithKeyMap(conversationFormKeyMap(translator)).
		RunWithContext(ctx)
	clearRenderedView(os.Stderr)
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}

type deleteAwareForm struct {
	form            *huh.Form
	field           *huh.Select[string]
	deleteRequested bool
	deleteValue     string
	filtering       bool
	exitRequested   bool
}

func (m *deleteAwareForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m *deleteAwareForm) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMessage, ok := message.(tea.KeyMsg); ok {
		switch keyMessage.Type {
		case tea.KeyEsc:
			if !m.filtering {
				m.exitRequested = true
				return m, tea.Quit
			}
			m.filtering = false
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyRunes:
			if !m.filtering && keyMessage.String() == "/" {
				m.filtering = true
			}
		}
		if !m.filtering && (keyMessage.Type == tea.KeyDelete || keyMessage.Type == tea.KeyBackspace) {
			if value, ok := m.field.Hovered(); ok && !isReservedSelection(value) {
				m.deleteRequested = true
				m.deleteValue = value
				return m, tea.Quit
			}
		}
	}
	updated, command := m.form.Update(message)
	if form, ok := updated.(*huh.Form); ok {
		m.form = form
	}
	return m, command
}

func (m *deleteAwareForm) View() string {
	return m.form.View()
}

func runConversationSelection(
	ctx context.Context,
	field *huh.Select[string],
	translator *catalog.Catalog,
) (string, bool, error) {
	if os.Getenv("TERM") == "dumb" {
		return "", false, runForm(ctx, translator, field)
	}
	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(huh.ThemeCharm()).
		WithKeyMap(conversationSelectKeyMap(translator))
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Interrupt

	model, err := tea.NewProgram(
		&deleteAwareForm{form: form, field: field},
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithReportFocus(),
	).Run()
	clearRenderedView(os.Stderr)
	result, ok := model.(*deleteAwareForm)
	if ok && result.deleteRequested {
		return result.deleteValue, true, nil
	}
	if ok && result.exitRequested {
		return "", false, nil
	}
	if ok && result.form.State == huh.StateAborted {
		return "", false, nil
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return "", false, nil
	}
	if errors.Is(err, tea.ErrProgramKilled) {
		return "", false, huh.ErrTimeout
	}
	if err != nil {
		return "", false, fmt.Errorf("huh: %w", err)
	}
	return "", false, nil
}

func conversationSelectKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	value := huh.NewDefaultKeyMap()
	compact := terminalWidth() < 60
	localizeConversationKeyMap(value, translator, compact)
	if compact {
		value.Select.Down.SetHelp("del", translator.Text("menu.help.compact_delete_down", nil))
	} else {
		value.Select.Down.SetHelp("↓", translator.Text("menu.help.delete_down", nil))
	}
	value.Select.Next.SetHelp("enter", translator.Text("menu.help.select_exit", nil))
	value.Select.Submit.SetHelp("enter", translator.Text("menu.help.select_exit", nil))
	return value
}

func conversationFormKeyMap(translator *catalog.Catalog) *huh.KeyMap {
	value := huh.NewDefaultKeyMap()
	localizeConversationKeyMap(value, translator, terminalWidth() < 60)
	nextHelp := translator.Text("menu.help.next_back_quit", nil)
	if terminalWidth() < 60 {
		nextHelp = translator.Text("menu.help.next", nil)
	}
	value.Input.Next.SetHelp("enter", nextHelp)
	value.Input.Submit.SetHelp("enter", nextHelp)
	value.Confirm.Next.SetHelp("enter", nextHelp)
	value.Confirm.Submit.SetHelp("enter", nextHelp)
	value.Select.Next.SetHelp("enter", nextHelp)
	value.Select.Submit.SetHelp("enter", nextHelp)
	return value
}

func localizeConversationKeyMap(value *huh.KeyMap, translator *catalog.Catalog, compact bool) {
	value.Input.AcceptSuggestion.SetHelp("ctrl+e", translator.Text("menu.help.complete", nil))
	value.Confirm.Toggle.SetHelp("←/→", translator.Text("menu.help.toggle", nil))
	if compact {
		value.Select.Up.SetHelp("", "")
		value.Select.Down.SetHelp("", "")
		value.Select.Filter.SetHelp("", "")
		return
	}
	value.Select.Up.SetHelp("↑", translator.Text("menu.help.move", nil))
	value.Select.Down.SetHelp("↓", translator.Text("menu.help.move", nil))
	value.Select.Filter.SetHelp("/", translator.Text("menu.help.search", nil))
}

func terminalWidth() int {
	if width, _, err := xterm.GetSize(os.Stderr.Fd()); err == nil {
		return width
	}
	return 80
}

func clearRenderedView(writer io.Writer) {
	_, _ = io.WriteString(writer, "\x1b[2J\x1b[H")
}

func isReservedSelection(value string) bool {
	return value == actionNew || value == actionBack
}
