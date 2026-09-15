package ask

import (
	"strings"
	"testing"
)

func TestEstimateDisplayCapacity(t *testing.T) {
	if got := EstimateDisplayCapacity(80, 24); got != 1344 {
		t.Fatalf("EstimateDisplayCapacity(80, 24) = %d, want 1344", got)
	}
	if got := EstimateDisplayCapacity(120, 40); got != 3552 {
		t.Fatalf("EstimateDisplayCapacity(120, 40) = %d, want 3552", got)
	}
	if got := EstimateDisplayCapacity(10, 24); got != 0 {
		t.Fatalf("small terminal capacity = %d, want 0", got)
	}
}

func TestWithDisplayBudgetAppendsAtPromptEnd(t *testing.T) {
	got := withDisplayBudget("question", 1344)
	if !strings.HasPrefix(got, "question\n\n[Terminal display budget:") {
		t.Fatalf("prompt = %q", got)
	}
	if !strings.Contains(got, "1344 character cells") ||
		!strings.HasSuffix(got, "long or comprehensive response.]") {
		t.Fatalf("prompt = %q", got)
	}
	if unchanged := withDisplayBudget("question", 0); unchanged != "question" {
		t.Fatalf("prompt without budget = %q", unchanged)
	}
}
