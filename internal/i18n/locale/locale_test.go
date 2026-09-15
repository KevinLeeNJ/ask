package locale

import "testing"

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"zh_CN.UTF-8": Chinese,
		"zh-CN":       Chinese,
		"en_US.utf8":  English,
		"en-US":       English,
		"C":           "",
		"POSIX":       "",
		"":            "",
		"fr_FR.UTF-8": "",
	}
	for input, want := range tests {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolvePriority(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := Resolve("en-US", "zh-CN"); got != English {
		t.Fatalf("CLI override = %q", got)
	}
	if got := Resolve("auto", "zh-CN"); got != Chinese {
		t.Fatalf("configured override = %q", got)
	}
	if got := Resolve("auto", "auto"); got != Chinese {
		t.Fatalf("LC_ALL = %q", got)
	}
}
