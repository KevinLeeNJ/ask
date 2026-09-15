package managedblock

import (
	"strings"
	"testing"
)

func TestUpsertIsIdempotentAndPreservesOtherSecrets(t *testing.T) {
	line := func(value string) string { return "export KEY='" + value + "'" }
	variable := func(value string) string {
		value = strings.TrimSpace(value)
		value = strings.TrimPrefix(value, "export ")
		name, _, _ := strings.Cut(value, "=")
		return name
	}

	content, _, err := Upsert("before\n", "KEY", line("one"), variable)
	if err != nil {
		t.Fatal(err)
	}
	content, _, err = Upsert(content, "OTHER", "export OTHER='two'", variable)
	if err != nil {
		t.Fatal(err)
	}
	content, _, err = Upsert(content, "KEY", line("three"), variable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "one") || !strings.Contains(content, "three") || !strings.Contains(content, "OTHER") {
		t.Fatalf("unexpected content:\n%s", content)
	}
	if strings.Count(content, StartMarker) != 1 || strings.Count(content, EndMarker) != 1 {
		t.Fatalf("marker count invalid:\n%s", content)
	}
}

func TestUpsertRejectsBrokenMarkers(t *testing.T) {
	_, _, err := Upsert(StartMarker+"\n", "KEY", "export KEY='x'", func(string) string { return "" })
	if err == nil {
		t.Fatal("expected broken marker error")
	}
}
