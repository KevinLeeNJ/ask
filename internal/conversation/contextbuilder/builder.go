package contextbuilder

import (
	"strings"
	"unicode/utf8"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

type Options struct {
	SystemPrompt   string
	Question       string
	History        []conversation.Message
	MaxMessages    int
	MaxTokens      int
	EstimateTokens func(string) int
}

func Build(options Options) ([]provider.Message, error) {
	estimate := options.EstimateTokens
	if estimate == nil {
		estimate = EstimateTokens
	}
	if options.MaxMessages > 0 && len(options.History) > options.MaxMessages {
		options.History = options.History[len(options.History)-options.MaxMessages:]
	}

	question := strings.TrimSpace(options.Question)
	questionTokens := estimate(question)
	systemTokens := estimate(options.SystemPrompt)
	if options.MaxTokens > 0 && systemTokens+questionTokens > options.MaxTokens {
		return nil, failure.New(
			failure.KindConfig,
			"context.question_too_large",
			map[string]string{
				"question_tokens": intString(questionTokens),
				"max_tokens":      intString(options.MaxTokens),
			},
		)
	}

	history := make([]provider.Message, 0, len(options.History)+1)
	total := systemTokens + questionTokens
	start := len(options.History)
	for start > 0 {
		message := options.History[start-1]
		if message.Role != string(provider.RoleUser) && message.Role != string(provider.RoleAssistant) {
			start--
			continue
		}
		if strings.TrimSpace(message.Content) == "" {
			start--
			continue
		}
		messageTokens := estimate(message.Content)
		if options.MaxTokens > 0 && total+messageTokens > options.MaxTokens {
			break
		}
		total += messageTokens
		start--
	}
	for _, message := range options.History[start:] {
		if message.Role != string(provider.RoleUser) && message.Role != string(provider.RoleAssistant) {
			continue
		}
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		history = append(history, provider.Message{
			Role:    provider.Role(message.Role),
			Content: message.Content,
		})
	}
	history = append(history, provider.Message{Role: provider.RoleUser, Content: options.Question})
	return history, nil
}

func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	tokens := (runes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}
