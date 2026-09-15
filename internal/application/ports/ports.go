package ports

import (
	"context"
	"net/http"
	"time"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
)

type ConfigStore interface {
	Load(ctx context.Context) (settings.Config, error)
	Save(ctx context.Context, config settings.Config) error
}

type Secret struct {
	Value    string
	EnvVar   string
	Provider provider.ID
}

type SecretResolver interface {
	Resolve(ctx context.Context, providerID provider.ID) (Secret, error)
}

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type ProviderHTTPConfig struct {
	ProviderID      provider.ID
	Format          provider.Format
	BaseURL         string
	Headers         map[string]string
	SessionIDHeader string
	Client          HTTPDoer
}

func (c ProviderHTTPConfig) RequestHeaders(sessionID string) map[string]string {
	headers := make(map[string]string, len(c.Headers)+1)
	for name, value := range c.Headers {
		headers[name] = value
	}
	if c.SessionIDHeader != "" && sessionID != "" {
		headers[c.SessionIDHeader] = sessionID
	}
	return headers
}

type ChatProvider interface {
	Complete(ctx context.Context, request provider.Request) (provider.Response, error)
	Stream(ctx context.Context, request provider.Request) (Stream, error)
}

type Stream interface {
	Recv() (stream.Event, error)
	Close() error
}

type ModelCatalog interface {
	ListModels(ctx context.Context, route provider.Route) ([]provider.Model, error)
}

type ResolvedProvider struct {
	Chat    ChatProvider
	Catalog ModelCatalog
}

type ProviderResolver interface {
	Resolve(profile settings.Provider, secret Secret) (ResolvedProvider, error)
}

type ShellProfileChange struct {
	Shell        string
	ProfileFile  string
	Variable     string
	Value        string
	ManagedBlock bool
	Overwrite    bool
}

type ShellProfilePreview struct {
	Shell       string
	Syntax      string
	ProfileFile string
	BackupFile  string
	Content     string
	Conflict    bool
	Source      string
}

type ShellProfileDefaults struct {
	Shell       string
	Syntax      string
	ProfileFile string
}

type ShellProfileWriter interface {
	Defaults() (ShellProfileDefaults, error)
	Preview(change ShellProfileChange) (ShellProfilePreview, error)
	Write(change ShellProfileChange) (ShellProfilePreview, error)
	Remove(change ShellProfileChange) (ShellProfilePreview, error)
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	New() (string, error)
}

type Presenter interface {
	Progress(event stream.Progress)
	ContentDelta(text string)
	ReasoningDelta(text string)
	Complete(response provider.Response)
	Fail(err error)
}

type ConversationRepository interface {
	Current(ctx context.Context) (conversation.Conversation, error)
	Latest(ctx context.Context) (conversation.Conversation, error)
	Get(ctx context.Context, id conversation.ID) (conversation.Conversation, error)
	FindByPrefix(ctx context.Context, prefix string) (conversation.Conversation, error)
	Create(ctx context.Context, value conversation.Conversation) error
	SetCurrent(ctx context.Context, id conversation.ID) error
	List(ctx context.Context, limit int) ([]conversation.Conversation, error)
	Messages(ctx context.Context, id conversation.ID, limit int) ([]conversation.Message, error)
	AppendUserAndClaimTitle(
		ctx context.Context,
		message conversation.Message,
		now int64,
	) (conversation.AppendResult, error)
	SaveMessage(ctx context.Context, message conversation.Message) error
	UpdateTitleIfNotUser(
		ctx context.Context,
		id conversation.ID,
		title string,
		source conversation.TitleSource,
	) (bool, error)
	Rename(ctx context.Context, id conversation.ID, title string) error
	Delete(ctx context.Context, id conversation.ID) error
}

type ConversationLeaser interface {
	AcquireLease(ctx context.Context, lease conversation.Lease) error
	ReleaseLease(ctx context.Context, id conversation.ID, owner string) error
}

type ConversationRetention interface {
	Cleanup(ctx context.Context, plan conversation.RetentionPlan) (conversation.RetentionResult, error)
	LastCleanupAt(ctx context.Context) (int64, error)
	MarkCleanupAt(ctx context.Context, unix int64) error
}

type ModelRouteStore interface {
	AddRecentRoute(ctx context.Context, route provider.Route, limit int) error
	RecentRoutes(ctx context.Context, limit int) ([]provider.Route, error)
}

type ModelCapabilityResolver interface {
	MinimumReasoningEffort(
		ctx context.Context,
		route provider.Route,
		profile settings.Provider,
	) (string, bool, error)
}

type ModelCapabilityCache interface {
	ModelCapability(ctx context.Context, key string) (string, bool, error)
	SaveModelCapability(ctx context.Context, key string, value string) error
}
