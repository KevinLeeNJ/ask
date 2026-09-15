package contextbuilder

import (
	"testing"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
)

func TestBuildKeepsSystemAndCurrentQuestion(t *testing.T) {
	history := []conversation.Message{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "recent question"},
		{Role: "assistant", Content: "recent answer"},
	}
	messages, err := Build(Options{
		SystemPrompt:   "system",
		Question:       "current",
		History:        history,
		MaxMessages:    2,
		MaxTokens:      1000,
		EstimateTokens: func(string) int { return 1 },
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content != "recent question" || messages[1].Content != "recent answer" || messages[2].Content != "current" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestBuildRejectsOversizedQuestion(t *testing.T) {
	_, err := Build(Options{
		Question:       "question",
		MaxTokens:      2,
		EstimateTokens: func(string) int { return 2 },
	})
	if err == nil {
		t.Fatal("expected oversized question error")
	}
}
