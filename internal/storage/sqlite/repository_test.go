package sqlite

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

func TestDefaultPathUsesHomeConfigDirectory(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("home config directory rule applies to macOS and Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, ".config", "ask", "ask.db")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestConversationLifecycleAndTitleClaim(t *testing.T) {
	repository := openTestRepository(t)
	now := time.Unix(1_700_000_000, 0)

	first := conversation.Conversation{
		ID:                   "11111111-0000-7000-8000-000000000001",
		Title:                "first",
		TitleSource:          conversation.TitleProvisional,
		SystemPromptSnapshot: "system",
		Provider:             "work",
		Model:                "model-a",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	second := first
	second.ID = "22222222-0000-7000-8000-000000000002"
	second.Title = "second"
	second.UpdatedAt = now.Add(time.Minute)

	if err := repository.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if err := repository.Create(context.Background(), second); err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	latest, err := repository.Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("Current() = %s, want latest %s", latest.ID, second.ID)
	}
	if err := repository.SetCurrent(context.Background(), first.ID); err != nil {
		t.Fatalf("SetCurrent() error = %v", err)
	}
	current, err := repository.Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.ID != first.ID {
		t.Fatalf("Current() = %s", current.ID)
	}

	userMessage := conversation.Message{
		ID:             "msg-user-1",
		ConversationID: first.ID,
		Role:           "user",
		Content:        "How does context work?",
		Status:         conversation.StatusComplete,
		CreatedAt:      now.Add(2 * time.Minute),
	}
	appended, err := repository.AppendUserAndClaimTitle(
		context.Background(),
		userMessage,
		userMessage.CreatedAt.Unix(),
	)
	if err != nil {
		t.Fatalf("AppendUserAndClaimTitle() error = %v", err)
	}
	if !appended.TitleClaimed {
		t.Fatal("first user message did not claim title generation")
	}
	assistant := conversation.Message{
		ID:               "msg-assistant-1",
		ConversationID:   first.ID,
		Role:             "assistant",
		Content:          "answer",
		ReasoningContent: "reasoning",
		Status:           conversation.StatusComplete,
		Provider:         "work",
		Model:            "model-a",
		CreatedAt:        now.Add(3 * time.Minute),
	}
	if err := repository.SaveMessage(context.Background(), assistant); err != nil {
		t.Fatalf("SaveMessage() error = %v", err)
	}
	secondMessage := userMessage
	secondMessage.ID = "msg-user-2"
	secondMessage.Content = "follow up"
	secondMessage.CreatedAt = now.Add(4 * time.Minute)
	appended, err = repository.AppendUserAndClaimTitle(
		context.Background(),
		secondMessage,
		secondMessage.CreatedAt.Unix(),
	)
	if err != nil {
		t.Fatalf("AppendUserAndClaimTitle(second) error = %v", err)
	}
	if appended.TitleClaimed {
		t.Fatal("second user message claimed title generation again")
	}

	updated, err := repository.UpdateTitleIfNotUser(
		context.Background(),
		first.ID,
		"Generated title",
		conversation.TitleAgent,
	)
	if err != nil || !updated {
		t.Fatalf("UpdateTitleIfNotUser() = %v, %v", updated, err)
	}
	if err := repository.Rename(context.Background(), first.ID, "User title"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	updated, err = repository.UpdateTitleIfNotUser(
		context.Background(),
		first.ID,
		"Async title",
		conversation.TitleAgent,
	)
	if err != nil || updated {
		t.Fatalf("async update after rename = %v, %v", updated, err)
	}
	value, err := repository.Get(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value.Title != "User title" || value.TitleSource != conversation.TitleUser {
		t.Fatalf("conversation = %#v", value)
	}
	if value.MessageCount != 3 {
		t.Fatalf("message count = %d", value.MessageCount)
	}

	byPrefix, err := repository.FindByPrefix(context.Background(), string(first.ID)[:12])
	if err != nil {
		t.Fatalf("FindByPrefix() error = %v", err)
	}
	if byPrefix.ID != first.ID {
		t.Fatalf("FindByPrefix() = %s", byPrefix.ID)
	}
	messages, err := repository.Messages(context.Background(), first.ID, 10)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(messages) != 3 || messages[0].ID != userMessage.ID || messages[2].ID != secondMessage.ID {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestConcurrentFirstMessageHasSingleTitleClaim(t *testing.T) {
	repository := openTestRepository(t)
	now := time.Unix(1_700_000_100, 0)
	value := conversation.Conversation{
		ID:          "018f0000-0000-7000-8000-000000000010",
		Title:       "provisional",
		TitleSource: conversation.TitleProvisional,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repository.Create(context.Background(), value); err != nil {
		t.Fatal(err)
	}

	var (
		waitGroup sync.WaitGroup
		lock      sync.Mutex
		claimed   int
	)
	for index := 0; index < 10; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			message := conversation.Message{
				ID:             "message-" + string(rune('a'+index)),
				ConversationID: value.ID,
				Role:           "user",
				Content:        "question",
				Status:         conversation.StatusComplete,
				CreatedAt:      now.Add(time.Duration(index) * time.Millisecond),
			}
			result, err := repository.AppendUserAndClaimTitle(
				context.Background(),
				message,
				message.CreatedAt.Unix(),
			)
			if err != nil {
				t.Errorf("AppendUserAndClaimTitle() error = %v", err)
				return
			}
			if result.TitleClaimed {
				lock.Lock()
				claimed++
				lock.Unlock()
			}
		}(index)
	}
	waitGroup.Wait()
	if claimed != 1 {
		t.Fatalf("title claimed %d times, want 1", claimed)
	}
}

func TestRecentRoutesAreDeduplicatedAndLimited(t *testing.T) {
	repository := openTestRepository(t)
	for index := 0; index < 12; index++ {
		route := provider.Route{Provider: "work", Model: "model-" + string(rune('a'+index))}
		if err := repository.AddRecentRoute(context.Background(), route, 10); err != nil {
			t.Fatalf("AddRecentRoute() error = %v", err)
		}
	}
	duplicate := provider.Route{Provider: "work", Model: "model-e"}
	if err := repository.AddRecentRoute(context.Background(), duplicate, 10); err != nil {
		t.Fatal(err)
	}
	routes, err := repository.RecentRoutes(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 10 || routes[0] != duplicate {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestCleanupProtectsCurrentLatestAndLeasedConversations(t *testing.T) {
	repository := openTestRepository(t)
	now := time.Now().Truncate(time.Second)
	values := []conversation.Conversation{
		testConversation("00000000-0000-7000-8000-000000000001", now.Add(-30*24*time.Hour)),
		testConversation("00000000-0000-7000-8000-000000000002", now.Add(-20*24*time.Hour)),
		testConversation("00000000-0000-7000-8000-000000000003", now.Add(-15*24*time.Hour)),
		testConversation("00000000-0000-7000-8000-000000000004", now.Add(-time.Hour)),
	}
	for _, value := range values {
		if err := repository.Create(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.SetCurrent(context.Background(), values[1].ID); err != nil {
		t.Fatal(err)
	}
	oldMessage := conversation.Message{
		ID:             "old-message",
		ConversationID: values[0].ID,
		Role:           "user",
		Content:        "old",
		Status:         conversation.StatusComplete,
		CreatedAt:      values[0].UpdatedAt,
	}
	if err := repository.SaveMessage(context.Background(), oldMessage); err != nil {
		t.Fatal(err)
	}
	if err := repository.AcquireLease(context.Background(), conversation.Lease{
		ConversationID: values[2].ID,
		Owner:          "owner",
		ExpiresAt:      now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := repository.Cleanup(context.Background(), conversation.RetentionPlan{
		Cutoff: now.Add(-7 * 24 * time.Hour),
		Limit:  500,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", result.Deleted)
	}
	if _, err := repository.Get(context.Background(), values[0].ID); failure.Info(err).MessageID != "conversation.not_found" {
		t.Fatalf("deleted conversation lookup error = %v", err)
	}
	for _, index := range []int{1, 2, 3} {
		if _, err := repository.Get(context.Background(), values[index].ID); err != nil {
			t.Fatalf("protected conversation %d error = %v", index, err)
		}
	}
	orphanCount := 0
	if err := repository.db.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`,
		string(values[0].ID),
	).Scan(&orphanCount); err != nil {
		t.Fatal(err)
	}
	if orphanCount != 0 {
		t.Fatalf("orphan messages = %d", orphanCount)
	}
	if err := repository.MarkCleanupAt(context.Background(), now.Unix()); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.LastCleanupAt(context.Background()); err != nil || got != now.Unix() {
		t.Fatalf("LastCleanupAt() = %d, %v", got, err)
	}
}

func openTestRepository(t *testing.T) *Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ask.db")
	repository, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return repository
}

func testConversation(id string, updatedAt time.Time) conversation.Conversation {
	return conversation.Conversation{
		ID:          conversation.ID(id),
		Title:       id,
		TitleSource: conversation.TitleProvisional,
		CreatedAt:   updatedAt.Add(-time.Hour),
		UpdatedAt:   updatedAt,
	}
}
