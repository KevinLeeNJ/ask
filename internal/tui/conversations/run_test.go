package conversations

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
)

func TestClearRenderedViewRemovesPreviousForm(t *testing.T) {
	var output bytes.Buffer
	clearRenderedView(&output)
	if got, want := output.String(), "\x1b[2J\x1b[H"; got != want {
		t.Fatalf("clear sequence = %q, want %q", got, want)
	}
}

func TestSelectionOptionsKeepActionsOutOfConversationList(t *testing.T) {
	values := []conversation.Conversation{
		{
			ID:           "conversation-1",
			Title:        "First conversation",
			MessageCount: 4,
			UpdatedAt:    time.Now(),
		},
	}

	options := selectionOptions(values, catalog.New("en-US"), 80)
	if len(options) != 3 {
		t.Fatalf("option count = %d, want 3", len(options))
	}
	if options[0].Value != "conversation-1" {
		t.Fatalf("first option value = %q", options[0].Value)
	}
	if options[1].Value != actionNew || options[2].Value != actionBack {
		t.Fatalf("action options = %#v", options[1:])
	}
}

func TestConversationSelectionRestoresEntry(t *testing.T) {
	values := []conversation.Conversation{
		{ID: "conversation-1"},
		{ID: "conversation-2"},
	}
	if got := initialConversationSelection(values); got != "conversation-1" {
		t.Fatalf("initial selection = %q, want conversation-1", got)
	}
	if !conversationSelectionExists(values, "conversation-2") {
		t.Fatal("existing conversation was not recognized")
	}
	if conversationSelectionExists(values, "deleted") {
		t.Fatal("deleted conversation was recognized")
	}
	if !conversationSelectionExists(values, actionNew) || !conversationSelectionExists(values, actionBack) {
		t.Fatal("reserved actions were not recognized")
	}
}

func TestConversationColumnsAreLocalized(t *testing.T) {
	chinese := catalog.New("zh-CN")
	english := catalog.New("en-US")
	if got := strings.Join(strings.Fields(conversationColumns(chinese, 80)), " "); got != "标题 更新时间 消息数" {
		t.Fatalf("Chinese columns = %q", conversationColumns(chinese, 80))
	}
	if got := strings.Join(strings.Fields(conversationColumns(english, 80)), " "); got != "Title Updated Messages" {
		t.Fatalf("English columns = %q", conversationColumns(english, 80))
	}
}

func TestConversationColumnsAlignWithRows(t *testing.T) {
	translator := catalog.New("zh-CN")
	header := conversationColumns(translator, 80)
	row := formatConversation(conversation.Conversation{
		Title:        "linux如何自定义 pagefault handler",
		MessageCount: 21,
		UpdatedAt:    time.Now().Add(-10 * time.Hour),
	}, translator, 80)

	if got, want := columnOffset("  "+row, "10 小时前"), columnOffset(header, "更新时间"); got != want {
		t.Fatalf("updated column offset = %d, want %d", got, want)
	}
	if got, want := columnOffset("  "+row, "21"), columnOffset(header, "消息数"); got != want {
		t.Fatalf("messages column offset = %d, want %d", got, want)
	}
}

func TestCompactConversationColumnsFitTerminal(t *testing.T) {
	translator := catalog.New("zh-CN")
	header := conversationColumns(translator, 50)
	row := formatConversation(conversation.Conversation{
		Title:        "linux如何自定义 pagefault handler",
		MessageCount: 21,
		UpdatedAt:    time.Now().Add(-10 * time.Hour),
	}, translator, 50)
	for name, value := range map[string]string{"header": header, "row": row} {
		if got := ansi.StringWidth(value); got > 50 {
			t.Fatalf("%s width = %d, want <= 50: %q", name, got, value)
		}
	}
}

func columnOffset(line, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	return ansi.StringWidth(line[:index])
}

func TestDeleteKeySelectsHoveredConversation(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(
			huh.NewOption("First", "conversation-1"),
			huh.NewOption("Second", "conversation-2"),
		).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if !model.deleteRequested {
		t.Fatal("delete key did not request deletion")
	}
	if model.deleteValue != "conversation-1" {
		t.Fatalf("delete value = %q", model.deleteValue)
	}
}

func TestDeleteKeyIgnoresReservedActions(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("+ New conversation", actionNew)).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if model.deleteRequested {
		t.Fatalf("reserved action %q triggered deletion", model.deleteValue)
	}
}

func TestBackspaceDeletesConversationOutsideFiltering(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("First", "conversation-1")).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !model.deleteRequested || model.deleteValue != "conversation-1" {
		t.Fatalf("delete state = requested:%v value:%q", model.deleteRequested, model.deleteValue)
	}
}

func TestBackspaceDuringFilteringEditsFilter(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("First", "conversation-1")).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if model.deleteRequested {
		t.Fatalf("backspace during filtering deleted %q", model.deleteValue)
	}
}

func TestEscapeExitsConversationMenu(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("First", "conversation-1")).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !model.exitRequested {
		t.Fatal("escape did not exit conversation menu")
	}
}

func TestEscapeClosesFilterBeforeExiting(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("First", "conversation-1")).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.exitRequested {
		t.Fatal("escape exited while closing filter")
	}
	if model.filtering {
		t.Fatal("escape did not close filter")
	}
}

func TestConversationListWrapsAtEdges(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(
			huh.NewOption("First", "conversation-1"),
			huh.NewOption("Second", "conversation-2"),
			huh.NewOption("Last", "conversation-3"),
		).
		Value(&selected)
	model := &deleteAwareForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if selected != "conversation-3" {
		t.Fatalf("up past start selected %q, want last", selected)
	}
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if selected != "conversation-1" {
		t.Fatalf("down past end selected %q, want first", selected)
	}
}

func TestConversationSelectKeyMapShowsDelete(t *testing.T) {
	keymap := conversationSelectKeyMap(catalog.New("zh-CN"))
	if help := keymap.Select.Down.Help(); help.Key != "↓" || help.Desc != "下移 · Delete/Backspace 删除" {
		t.Fatalf("down help = %#v", help)
	}
	if help := keymap.Select.Next.Help(); help.Key != "enter" || help.Desc != "选择 · esc 退出" {
		t.Fatalf("select help = %#v", help)
	}
}
