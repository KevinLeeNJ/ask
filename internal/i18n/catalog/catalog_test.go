package catalog

import (
	"reflect"
	"testing"

	"github.com/KevinLeeNJ/ask/internal/i18n/locale"
)

func TestLanguagePacksHaveSameKeys(t *testing.T) {
	englishKeys := Keys(locale.English)
	chineseKeys := Keys(locale.Chinese)
	if !reflect.DeepEqual(englishKeys, chineseKeys) {
		t.Fatalf("language pack keys differ:\nenglish=%v\nchinese=%v", englishKeys, chineseKeys)
	}
}

func TestChinesePackDoesNotFallBackToEnglishUserCopy(t *testing.T) {
	allowedIdentical := map[string]bool{
		"status.provider": true,
		"version.text":    true,
	}
	for key, englishText := range english {
		if allowedIdentical[key] {
			continue
		}
		if chinese[key] == englishText {
			t.Errorf("Chinese message %q still uses English copy", key)
		}
	}
}

func TestChineseMenuDoesNotFallBackToEnglishUserCopy(t *testing.T) {
	for key, englishText := range englishMenu {
		if chineseMenu[key] == englishText {
			t.Errorf("Chinese menu message %q still uses English copy", key)
		}
	}
}

func TestTextFormatsArguments(t *testing.T) {
	translator := New(locale.Chinese)
	got := translator.Text("config.active_provider_unknown", map[string]string{"provider": "work"})
	want := "active_provider 指向的 `work` 不存在"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}
