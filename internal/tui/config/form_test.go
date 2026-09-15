package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
)

func TestClearRenderedViewRemovesPreviousForm(t *testing.T) {
	var output bytes.Buffer
	clearRenderedView(&output)
	if got, want := output.String(), "\x1b[2J\x1b[H"; got != want {
		t.Fatalf("clear sequence = %q, want %q", got, want)
	}
}

func TestFittedFormHeightLeavesBottomRowAvailable(t *testing.T) {
	tests := []struct {
		terminal int
		want     int
	}{
		{terminal: 0, want: 0},
		{terminal: 3, want: 3},
		{terminal: 4, want: 4},
		{terminal: 5, want: 4},
		{terminal: 30, want: 29},
	}
	for _, test := range tests {
		if got := fittedFormHeight(test.terminal); got != test.want {
			t.Fatalf("fittedFormHeight(%d) = %d, want %d", test.terminal, got, test.want)
		}
	}
}

func TestKeyMapOnlyBindsEscapeToBack(t *testing.T) {
	keymap := keyMap(catalog.New("zh-CN"))
	if !key.Matches(tea.KeyMsg{Type: tea.KeyEsc}, keymap.Quit) {
		t.Fatal("esc is not bound to back")
	}
	if key.Matches(tea.KeyMsg{Type: tea.KeyCtrlC}, keymap.Quit) {
		t.Fatal("ctrl+c must not use the form back binding")
	}
	want := "下一步 · esc 返回 · ctrl+c 退出"
	if got := keymap.Input.Next.Help().Desc; got != want {
		t.Fatalf("input next help = %q, want %q", got, want)
	}
	if got := keymap.Input.Submit.Help().Desc; got != want {
		t.Fatalf("input submit help = %q, want %q", got, want)
	}
}

func TestRootKeyMapShowsEscapeExit(t *testing.T) {
	keymap := rootKeyMap(catalog.New("zh-CN"))
	want := "选择 · esc 退出 · ctrl+c 退出"
	if got := keymap.Select.Next.Help().Desc; got != want {
		t.Fatalf("select next help = %q, want %q", got, want)
	}
	if got := keymap.Select.Submit.Help().Desc; got != want {
		t.Fatalf("select submit help = %q, want %q", got, want)
	}
}

func TestRootKeyMapLocalizesDefaultListHelp(t *testing.T) {
	selected := ""
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Options(huh.NewOption("first", "first"), huh.NewOption("second", "second")).
			Value(&selected),
	)).WithKeyMap(rootKeyMap(catalog.New("zh-CN")))
	help := form.Help()
	help.Width = 240
	footer := help.ShortHelpView(form.KeyBinds())
	for _, want := range []string{"↑ 移动", "↓ 移动", "/ 搜索", "enter 选择 · esc 退出"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q: %q", want, footer)
		}
	}
	for _, unwanted := range []string{" up", " down", " filter"} {
		if strings.Contains(footer, unwanted) {
			t.Fatalf("footer still contains English default %q: %q", unwanted, footer)
		}
	}
}

func TestCompactFormKeyMapKeepsLocalizedNavigationHelp(t *testing.T) {
	value := formKeyMap("下一步", "返回", catalog.New("zh-CN"), true)
	if got := value.Select.Up.Help(); got.Key != "↑" || got.Desc != "移动" {
		t.Fatalf("compact up help = %#v", got)
	}
	if got := value.Select.Filter.Help(); got.Key != "/" || got.Desc != "搜索" {
		t.Fatalf("compact filter help = %#v", got)
	}
}

func TestModelKeyMapShowsAccurateModelControls(t *testing.T) {
	keymap := modelKeyMap(catalog.New("zh-CN"))
	tests := []struct {
		name string
		got  key.Help
		key  string
		desc string
	}{
		{name: "toggle", got: keymap.MultiSelect.Toggle.Help(), key: "space/x", desc: "切换"},
		{name: "up", got: keymap.MultiSelect.Up.Help(), key: "↑/k", desc: "移动"},
		{name: "down", got: keymap.MultiSelect.Down.Help(), key: "↓/j", desc: "移动"},
		{name: "search", got: keymap.MultiSelect.Filter.Help(), key: "/", desc: "搜索"},
		{name: "submit", got: keymap.MultiSelect.Submit.Help(), key: "enter", desc: "确认 · esc 返回 · ctrl+c 退出"},
	}
	for _, test := range tests {
		if test.got.Key != test.key || test.got.Desc != test.desc {
			t.Fatalf("%s help = %#v, want key %q desc %q", test.name, test.got, test.key, test.desc)
		}
	}
}

func TestSystemPromptKeyMapShowsSave(t *testing.T) {
	keymap := systemPromptKeyMap(catalog.New("zh-CN"))
	if !key.Matches(tea.KeyMsg{Type: tea.KeyCtrlS}, keymap.Text.Submit) {
		t.Fatal("ctrl+s is not bound to system prompt save")
	}
	if help := keymap.Text.Submit.Help(); help.Key != "ctrl+s" || help.Desc != "保存" {
		t.Fatalf("save help = %#v", help)
	}
}

func TestModelFooterShowsAccurateModelControls(t *testing.T) {
	selected := []string{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Options(huh.NewOption("model-a", "model-a")).
			Filterable(true).
			Value(&selected),
	)).WithKeyMap(modelKeyMap(catalog.New("zh-CN")))
	help := form.Help()
	help.Width = 240
	footer := help.ShortHelpView(form.KeyBinds())
	for _, want := range []string{
		"space/x 切换",
		"↑/k 移动",
		"↓/j 移动",
		"/ 搜索",
		"enter 确认 · esc 返回 · ctrl+c 退出",
	} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q: %q", want, footer)
		}
	}
}

func TestGlobalQuitFormWrapsSelectAtListEdges(t *testing.T) {
	selected := ""
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Options(
				huh.NewOption("first", "first"),
				huh.NewOption("second", "second"),
				huh.NewOption("last", "last"),
			).
			Value(&selected),
	)).WithTheme(huh.ThemeCharm())
	model := &globalQuitForm{form: form}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if selected != "last" {
		t.Fatalf("up past start selected %q, want last", selected)
	}
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if selected != "first" {
		t.Fatalf("down past end selected %q, want first", selected)
	}
}

func TestGlobalQuitFormInterceptsCtrlC(t *testing.T) {
	form := testForm()
	model := &globalQuitForm{form: form}

	_, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if !model.exitRequested {
		t.Fatal("ctrl+c did not request application exit")
	}
	if form.State != huh.StateNormal {
		t.Fatalf("ctrl+c changed form state to %v", form.State)
	}
	if command == nil {
		t.Fatal("ctrl+c did not return a quit command")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c command = %T, want tea.QuitMsg", command())
	}
}

func TestGlobalQuitFormPassesEscapeToForm(t *testing.T) {
	form := testForm()
	form.CancelCmd = tea.Interrupt
	model := &globalQuitForm{form: form}

	_, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if model.exitRequested {
		t.Fatal("esc requested application exit")
	}
	if form.State != huh.StateAborted {
		t.Fatalf("esc form state = %v, want %v", form.State, huh.StateAborted)
	}
	if command == nil {
		t.Fatal("esc did not return the cancel command")
	}
	if _, ok := command().(tea.InterruptMsg); !ok {
		t.Fatalf("esc command = %T, want tea.InterruptMsg", command())
	}
}

func TestProviderSelectFormDeletesHoveredProvider(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(
			huh.NewOption("work", "work"),
			huh.NewOption("local", "local"),
		).
		Value(&selected)
	model := &providerSelectForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if !model.deleteRequested || model.deleteValue != "work" {
		t.Fatalf("delete state = requested:%v value:%q", model.deleteRequested, model.deleteValue)
	}
}

func TestProviderSelectFormIgnoresReservedActions(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("+ Add provider", providerActionAdd)).
		Value(&selected)
	model := &providerSelectForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if model.deleteRequested {
		t.Fatalf("reserved action %q triggered deletion", model.deleteValue)
	}
}

func TestProviderSelectFormWrapsAtEdges(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(
			huh.NewOption("work", "work"),
			huh.NewOption("local", "local"),
		).
		Value(&selected)
	model := &providerSelectForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if selected != "local" {
		t.Fatalf("up past start selected %q, want local", selected)
	}
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if selected != "work" {
		t.Fatalf("down past end selected %q, want work", selected)
	}
}

func TestProviderSelectFormBackspaceDeletesHoveredProvider(t *testing.T) {
	selected := ""
	field := huh.NewSelect[string]().
		Options(huh.NewOption("work", "work")).
		Value(&selected)
	model := &providerSelectForm{
		form:  huh.NewForm(huh.NewGroup(field)).WithTheme(huh.ThemeCharm()),
		field: field,
	}

	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !model.deleteRequested || model.deleteValue != "work" {
		t.Fatalf("delete state = requested:%v value:%q", model.deleteRequested, model.deleteValue)
	}
}

func TestProviderSelectKeyMapShowsDelete(t *testing.T) {
	keymap := providerSelectKeyMap(catalog.New("zh-CN"))
	if help := keymap.Select.Down.Help(); help.Key != "↓" || help.Desc != "下移 · Delete/Backspace 删除" {
		t.Fatalf("down help = %#v", help)
	}
	if help := keymap.Select.Next.Help(); help.Key != "enter" || help.Desc != "编辑 · esc 退出" {
		t.Fatalf("next help = %#v", help)
	}
	if help := keymap.Quit.Help(); help.Key != "esc" || help.Desc != "返回" {
		t.Fatalf("quit help = %#v", help)
	}
}

func TestRunSubmenuEscapeOnMenuReturnsToParent(t *testing.T) {
	showCalls := 0
	editCalls := 0
	err := runSubmenu(
		func() error {
			showCalls++
			return errCancelled
		},
		func() error {
			editCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("runSubmenu() error = %v", err)
	}
	if showCalls != 1 || editCalls != 0 {
		t.Fatalf("calls = show:%d edit:%d, want show:1 edit:0", showCalls, editCalls)
	}
}

func TestRunSubmenuEscapeOnFieldReturnsToSameMenu(t *testing.T) {
	showCalls := 0
	editCalls := 0
	err := runSubmenu(
		func() error {
			showCalls++
			return nil
		},
		func() error {
			editCalls++
			if editCalls == 1 {
				return errCancelled
			}
			return errSubmenuBack
		},
	)
	if err != nil {
		t.Fatalf("runSubmenu() error = %v", err)
	}
	if showCalls != 2 || editCalls != 2 {
		t.Fatalf("calls = show:%d edit:%d, want show:2 edit:2", showCalls, editCalls)
	}
}

func TestRunLoadingDumbTerminalReturnsWorkError(t *testing.T) {
	t.Setenv("TERM", "dumb")
	want := errors.New("failed")
	got := runLoading(context.Background(), "loading", func(context.Context) error {
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("runLoading() error = %v, want %v", got, want)
	}
}

func testForm() *huh.Form {
	value := ""
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Value(&value),
	)).WithKeyMap(keyMap(catalog.New("en-US")))
}
