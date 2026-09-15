package ask

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/config/validation"
	"github.com/KevinLeeNJ/ask/internal/conversation/contextbuilder"
	titlegen "github.com/KevinLeeNJ/ask/internal/conversation/title"
	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
	"github.com/KevinLeeNJ/ask/internal/domain/stream"
)

type Options struct {
	Question                   string
	ProviderOverride           string
	ModelOverride              string
	SystemOverride             *string
	ThinkingOverride           *provider.ThinkingMode
	TimeoutOverride            *time.Duration
	MaxContextMessagesOverride int
	ConversationSelector       string
	NewConversation            bool
	NoStream                   bool
	DefaultSystemZH            string
	DefaultSystemEN            string
	Language                   string
	DisplayCapacity            int
	StdinCharacters            int
}

type Result struct {
	ConversationID string
	MessageID      string
	Response       provider.Response
	Elapsed        time.Duration
}

type Dependencies struct {
	Configs       ports.ConfigStore
	Secrets       ports.SecretResolver
	Providers     ports.ProviderResolver
	Conversations ports.ConversationRepository
	Leases        ports.ConversationLeaser
	Retention     ports.ConversationRetention
	Routes        ports.ModelRouteStore
	Capabilities  ports.ModelCapabilityResolver
	Clock         ports.Clock
	IDs           ports.IDGenerator
}

type Service struct {
	deps      Dependencies
	present   ports.Presenter
	cleanupWG sync.WaitGroup
	titleWG   sync.WaitGroup
}

func NewService(dependencies Dependencies, presenter ports.Presenter) *Service {
	return &Service{deps: dependencies, present: presenter}
}

func (s *Service) Execute(ctx context.Context, options Options) (Result, error) {
	if strings.TrimSpace(options.Question) == "" {
		return Result{}, failure.New(failure.KindUsage, "usage.no_question", nil)
	}
	s.present.Progress(progress("preparing", 0))
	defer s.waitForCleanup()
	titleCtx, titleCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		titleCancel()
		s.waitForTitles()
	}()

	config, err := s.deps.Configs.Load(ctx)
	if err != nil {
		return Result{}, err
	}
	config, err = validation.Normalize(config)
	if err != nil {
		return Result{}, err
	}
	conversationValue, conversationExists, err := s.resolveConversationTarget(ctx, options)
	if err != nil {
		return Result{}, err
	}
	conversationRoute := provider.Route{}
	if conversationExists {
		conversationRoute = provider.Route{
			Provider: provider.ID(strings.TrimSpace(conversationValue.Provider)),
			Model:    strings.TrimSpace(conversationValue.Model),
		}
	}
	route, profile, err := resolveRoute(
		config,
		options.ProviderOverride,
		options.ModelOverride,
		conversationRoute,
	)
	if err != nil {
		return Result{}, err
	}
	secret, err := s.deps.Secrets.Resolve(ctx, route.Provider)
	if err != nil {
		return Result{}, err
	}
	resolved, err := s.deps.Providers.Resolve(profile, secret)
	if err != nil {
		return Result{}, err
	}

	thinking, err := resolveThinking(
		ctx,
		config.Reasoning,
		route,
		profile,
		options.ThinkingOverride,
		options.Question,
		options.StdinCharacters,
		s.deps.Capabilities,
	)
	if err != nil {
		return Result{}, err
	}
	systemPrompt := config.Chat.SystemPrompt
	if options.SystemOverride != nil {
		systemPrompt = *options.SystemOverride
	} else if strings.TrimSpace(systemPrompt) == "" {
		if options.Language == "zh-CN" {
			systemPrompt = options.DefaultSystemZH
		} else {
			systemPrompt = options.DefaultSystemEN
		}
	}

	if !conversationExists && s.deps.Conversations != nil {
		conversationValue, err = s.createConversation(ctx, options.Question, route, systemPrompt)
		if err != nil {
			return Result{}, err
		}
	}
	conversationID := conversationValue.ID
	releaseLease, err := s.acquireLease(ctx, conversationID, profile.RequestTimeout, options.TimeoutOverride)
	if err != nil {
		return Result{}, err
	}
	if releaseLease != nil {
		defer releaseLease()
	}

	messages, err := s.prepareMessages(
		ctx,
		conversationID,
		systemPrompt,
		options.Question,
		withDisplayBudget(options.Question, options.DisplayCapacity),
		config,
		options.MaxContextMessagesOverride,
		route,
		resolved.Chat,
		titleCtx,
	)
	if err != nil {
		return Result{}, err
	}

	timeout := profile.RequestTimeout
	if options.TimeoutOverride != nil {
		timeout = *options.TimeoutOverride
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.present.Progress(progress("connecting", 0))
	request := provider.Request{
		Route:           route,
		SessionID:       string(conversationID),
		SystemPrompt:    systemPrompt,
		Messages:        messages,
		Stream:          !options.NoStream && config.Chat.Stream,
		MaxOutputTokens: config.Model.MaxOutputTokens,
		Thinking:        thinking,
	}
	temperature := config.Model.Temperature
	request.Temperature = &temperature
	s.scheduleCleanup(config)

	started := time.Now()
	s.present.Progress(progress("waiting", 0))
	var (
		result  Result
		execErr error
	)
	if request.Stream {
		result, execErr = s.executeStream(requestCtx, cancel, resolved.Chat, request, started)
	} else {
		response, err := resolved.Chat.Complete(requestCtx, request)
		if err != nil {
			s.present.Fail(err)
			execErr = err
		} else {
			s.present.ContentDelta(response.Content)
			s.present.Complete(response)
			result = Result{Response: response, Elapsed: time.Since(started)}
		}
	}
	result.ConversationID = string(conversationID)
	if conversationID == "" {
		result.ConversationID = ""
	}

	persistCtx := ctx
	persistCancel := func() {}
	if execErr != nil {
		persistCtx, persistCancel = context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	}
	messageID := s.persistAssistant(persistCtx, conversationID, result.Response, execErr)
	persistCancel()
	if messageID != "" {
		result.MessageID = messageID
	}
	if execErr == nil && s.deps.Routes != nil {
		_ = s.deps.Routes.AddRecentRoute(ctx, route, 10)
	}
	return result, execErr
}

func (s *Service) resolveConversationTarget(
	ctx context.Context,
	options Options,
) (conversation.Conversation, bool, error) {
	if s.deps.Conversations == nil {
		return conversation.Conversation{}, false, nil
	}
	if options.NewConversation {
		return conversation.Conversation{}, false, nil
	}
	if selector := strings.TrimSpace(options.ConversationSelector); selector != "" {
		value, err := s.deps.Conversations.FindByPrefix(ctx, selector)
		if err != nil {
			return conversation.Conversation{}, false, err
		}
		return value, true, nil
	}
	current, err := s.deps.Conversations.Current(ctx)
	if err == nil {
		return current, true, nil
	}
	if failure.Info(err).MessageID == "conversation.not_found" {
		return conversation.Conversation{}, false, nil
	}
	return conversation.Conversation{}, false, err
}

func (s *Service) createConversation(
	ctx context.Context,
	question string,
	route provider.Route,
	systemPrompt string,
) (conversation.Conversation, error) {
	idValue, err := s.newID()
	if err != nil {
		return conversation.Conversation{}, err
	}
	now := s.now()
	value := conversation.Conversation{
		ID:                   conversation.ID(idValue),
		Title:                titlegen.Provisional(question),
		TitleSource:          conversation.TitleProvisional,
		SystemPromptSnapshot: systemPrompt,
		Provider:             string(route.Provider),
		Model:                route.Model,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.deps.Conversations.Create(ctx, value); err != nil {
		return conversation.Conversation{}, err
	}
	if err := s.deps.Conversations.SetCurrent(ctx, value.ID); err != nil {
		return conversation.Conversation{}, err
	}
	return value, nil
}

func (s *Service) acquireLease(
	ctx context.Context,
	conversationID conversation.ID,
	requestTimeout time.Duration,
	override *time.Duration,
) (func(), error) {
	if s.deps.Leases == nil || conversationID == "" {
		return nil, nil
	}
	owner, err := s.newID()
	if err != nil {
		return nil, err
	}
	if override != nil {
		requestTimeout = *override
	}
	ttl := 5 * time.Minute
	if requestTimeout+time.Minute > ttl {
		ttl = requestTimeout + time.Minute
	}
	lease := conversation.Lease{
		ConversationID: conversationID,
		Owner:          owner,
		ExpiresAt:      s.now().Add(ttl),
	}
	if err := s.deps.Leases.AcquireLease(ctx, lease); err != nil {
		return nil, err
	}
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				heartbeatCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				lease.ExpiresAt = s.now().Add(ttl)
				_ = s.deps.Leases.AcquireLease(heartbeatCtx, lease)
				cancel()
			case <-stopHeartbeat:
				return
			}
		}
	}()
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(stopHeartbeat)
			releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = s.deps.Leases.ReleaseLease(releaseCtx, conversationID, owner)
		})
	}
	return release, nil
}

func (s *Service) prepareMessages(
	ctx context.Context,
	conversationID conversation.ID,
	systemPrompt string,
	question string,
	promptQuestion string,
	config settings.Config,
	maxMessagesOverride int,
	route provider.Route,
	chat ports.ChatProvider,
	titleCtx context.Context,
) ([]provider.Message, error) {
	if conversationID == "" || s.deps.Conversations == nil {
		return []provider.Message{{Role: provider.RoleUser, Content: promptQuestion}}, nil
	}
	maxMessages := config.Chat.MaxContextMessages
	if maxMessagesOverride > 0 {
		maxMessages = maxMessagesOverride
	}
	history, err := s.deps.Conversations.Messages(ctx, conversationID, maxMessages)
	if err != nil {
		return nil, err
	}
	messages, err := contextbuilder.Build(contextbuilder.Options{
		SystemPrompt: systemPrompt,
		Question:     promptQuestion,
		History:      history,
		MaxMessages:  maxMessages,
		MaxTokens:    config.Chat.MaxContextTokens,
	})
	if err != nil {
		return nil, err
	}
	userID, err := s.newID()
	if err != nil {
		return nil, err
	}
	appended, err := s.deps.Conversations.AppendUserAndClaimTitle(ctx, conversation.Message{
		ID:             userID,
		ConversationID: conversationID,
		Role:           string(provider.RoleUser),
		Content:        question,
		Status:         conversation.StatusComplete,
		Provider:       string(route.Provider),
		Model:          route.Model,
		CreatedAt:      s.now(),
	}, s.now().Unix())
	if err != nil {
		return nil, err
	}
	if appended.TitleClaimed && chat != nil {
		s.titleWG.Add(1)
		go func() {
			defer s.titleWG.Done()
			s.generateTitle(titleCtx, appended.Conversation.ID, question, chat, route)
		}()
	}
	return messages, nil
}

func (s *Service) generateTitle(
	ctx context.Context,
	conversationID conversation.ID,
	question string,
	chat ports.ChatProvider,
	route provider.Route,
) {
	generator := titlegen.Generator{
		Client:    chat,
		Route:     route,
		SessionID: string(conversationID),
	}
	value, err := generator.Generate(ctx, question)
	if err != nil {
		return
	}
	_, _ = s.deps.Conversations.UpdateTitleIfNotUser(
		ctx,
		conversationID,
		value,
		conversation.TitleAgent,
	)
}

func (s *Service) waitForTitles() {
	done := make(chan struct{})
	go func() {
		s.titleWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *Service) persistAssistant(
	ctx context.Context,
	conversationID conversation.ID,
	response provider.Response,
	requestErr error,
) string {
	if s.deps.Conversations == nil || conversationID == "" {
		return ""
	}
	if strings.TrimSpace(response.Content) == "" && strings.TrimSpace(response.ReasoningContent) == "" {
		return ""
	}
	idValue, err := s.newID()
	if err != nil {
		return ""
	}
	status := conversation.StatusComplete
	errorText := ""
	if requestErr != nil {
		status = conversation.StatusPartial
		errorText = failure.Info(requestErr).MessageID
	}
	message := conversation.Message{
		ID:               idValue,
		ConversationID:   conversationID,
		Role:             string(provider.RoleAssistant),
		Content:          response.Content,
		ReasoningContent: response.ReasoningContent,
		Status:           status,
		Provider:         string(response.Provider),
		Model:            response.Model,
		InputTokens:      response.Usage.InputTokens,
		OutputTokens:     response.Usage.OutputTokens,
		ReasoningTokens:  response.Usage.ReasoningTokens,
		Error:            errorText,
		CreatedAt:        s.now(),
	}
	if err := s.deps.Conversations.SaveMessage(ctx, message); err != nil {
		return ""
	}
	return message.ID
}

func (s *Service) scheduleCleanup(config settings.Config) {
	if s.deps.Retention == nil || config.History.Retention == settings.RetentionNever {
		return
	}
	s.cleanupWG.Add(1)
	go func() {
		defer s.cleanupWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		last, err := s.deps.Retention.LastCleanupAt(ctx)
		if err != nil {
			return
		}
		now := s.now()
		if last > 0 && now.Unix()-last < int64((24*time.Hour).Seconds()) {
			return
		}
		days := map[string]int{
			settings.Retention7Days:  7,
			settings.Retention30Days: 30,
			settings.Retention60Days: 60,
		}[config.History.Retention]
		if days == 0 {
			return
		}
		if _, err := s.deps.Retention.Cleanup(ctx, conversation.RetentionPlan{
			Cutoff: now.Add(-time.Duration(days) * 24 * time.Hour),
			Limit:  500,
		}); err != nil {
			return
		}
		_ = s.deps.Retention.MarkCleanupAt(ctx, now.Unix())
	}()
}

func (s *Service) waitForCleanup() {
	done := make(chan struct{})
	go func() {
		s.cleanupWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *Service) now() time.Time {
	if s.deps.Clock != nil {
		return s.deps.Clock.Now()
	}
	return time.Now()
}

func (s *Service) newID() (string, error) {
	if s.deps.IDs != nil {
		return s.deps.IDs.New()
	}
	return "", failure.New(failure.KindInternal, "id.generate_failed", nil)
}

type receiveResult struct {
	event stream.Event
	err   error
}

func (s *Service) executeStream(
	ctx context.Context,
	cancel context.CancelFunc,
	providerClient ports.ChatProvider,
	request provider.Request,
	started time.Time,
) (Result, error) {
	providerStream, err := providerClient.Stream(ctx, request)
	if err != nil {
		s.present.Fail(err)
		return Result{}, err
	}
	defer providerStream.Close()

	events := make(chan receiveResult, 8)
	go func() {
		for {
			event, err := providerStream.Recv()
			select {
			case events <- receiveResult{event: event, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var content strings.Builder
	var reasoning strings.Builder
	response := provider.Response{
		Provider: request.Route.Provider,
		Model:    request.Route.Model,
	}
	stage := stream.StageWaiting
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case received := <-events:
			if received.err != nil {
				if errors.Is(received.err, io.EOF) {
					if content.Len() == 0 {
						err := failure.New(
							failure.KindProtocol,
							"upstream.empty_response",
							map[string]string{"provider": string(request.Route.Provider)},
						)
						s.present.Fail(err)
						return Result{Response: response}, err
					}
					response.Content = content.String()
					response.ReasoningContent = reasoning.String()
					s.present.Complete(response)
					return Result{Response: response, Elapsed: time.Since(started)}, nil
				}
				s.present.Fail(received.err)
				response.Content = content.String()
				response.ReasoningContent = reasoning.String()
				return Result{Response: response}, received.err
			}
			switch event := received.event.(type) {
			case stream.TextDelta:
				if content.Len() == 0 {
					stage = stream.StageGenerating
					s.present.Progress(progress(string(stage), time.Since(started)))
				}
				content.WriteString(event.Text)
				s.present.ContentDelta(event.Text)
			case stream.ReasoningDelta:
				if stage != stream.StageGenerating {
					stage = stream.StageThinking
					s.present.Progress(progress(string(stage), time.Since(started)))
				}
				reasoning.WriteString(event.Text)
				s.present.ReasoningDelta(event.Text)
			case stream.Usage:
				response.Usage = event.Usage
			case stream.Completed:
				if content.Len() == 0 {
					err := failure.New(
						failure.KindProtocol,
						"upstream.empty_response",
						map[string]string{"provider": string(request.Route.Provider)},
					)
					s.present.Fail(err)
					return Result{Response: response}, err
				}
				response.Content = content.String()
				response.ReasoningContent = reasoning.String()
				s.present.Complete(response)
				return Result{Response: response, Elapsed: time.Since(started)}, nil
			}
		case <-ticker.C:
			s.present.Progress(progress(string(stage), time.Since(started)))
		case <-ctx.Done():
			cancel()
			err := failure.Wrap(failure.KindCanceled, "request.canceled", nil, ctx.Err())
			s.present.Fail(err)
			return Result{
				Response: provider.Response{
					Content:          content.String(),
					ReasoningContent: reasoning.String(),
					Usage:            response.Usage,
					Provider:         response.Provider,
					Model:            response.Model,
				},
			}, err
		}
	}
}

func progress(stage string, elapsed time.Duration) stream.Progress {
	return stream.Progress{
		Stage:     stream.ProgressStage(stage),
		ElapsedMs: elapsed.Milliseconds(),
	}
}
