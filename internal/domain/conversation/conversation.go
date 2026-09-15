package conversation

import "time"

type ID string

type TitleSource string

const (
	TitleProvisional TitleSource = "provisional"
	TitleAgent       TitleSource = "agent"
	TitleUser        TitleSource = "user"
)

type Conversation struct {
	ID                   ID
	Title                string
	TitleSource          TitleSource
	TitleAttemptedAt     int64
	SystemPromptSnapshot string
	Provider             string
	Model                string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	MessageCount         int
}

type MessageStatus string

const (
	StatusComplete MessageStatus = "complete"
	StatusPartial  MessageStatus = "partial"
	StatusFailed   MessageStatus = "failed"
)

type Message struct {
	ID               string
	ConversationID   ID
	Role             string
	Content          string
	ReasoningContent string
	Status           MessageStatus
	Provider         string
	Model            string
	CreatedAt        time.Time
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	Error            string
}

type AppendResult struct {
	Conversation Conversation
	TitleClaimed bool
}

type Lease struct {
	ConversationID ID
	Owner          string
	ExpiresAt      time.Time
}

type RetentionPlan struct {
	Cutoff          time.Time
	CurrentID       ID
	LatestID        ID
	Limit           int
	ProtectedLeases []ID
}

type RetentionResult struct {
	Deleted int
}
