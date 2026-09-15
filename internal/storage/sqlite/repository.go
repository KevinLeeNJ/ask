package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

var _ ports.ConversationRepository = (*Repository)(nil)
var _ ports.ConversationLeaser = (*Repository)(nil)
var _ ports.ConversationRetention = (*Repository)(nil)
var _ ports.ModelRouteStore = (*Repository)(nil)
var _ ports.ModelCapabilityCache = (*Repository)(nil)

func (r *Repository) Current(ctx context.Context) (conversation.Conversation, error) {
	var current string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT value FROM app_state WHERE key = 'current_conversation_id'`,
	).Scan(&current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.Latest(ctx)
		}
		return conversation.Conversation{}, wrapStorage("current conversation", err)
	}
	if strings.TrimSpace(current) == "" {
		return r.Latest(ctx)
	}
	value, err := r.Get(ctx, conversation.ID(current))
	if err != nil {
		if failure.Info(err).MessageID == "conversation.not_found" {
			return r.Latest(ctx)
		}
		return conversation.Conversation{}, err
	}
	return value, nil
}

func (r *Repository) Latest(ctx context.Context) (conversation.Conversation, error) {
	value, err := scanConversation(r.db.QueryRowContext(
		ctx,
		conversationSelect()+` ORDER BY c.updated_at DESC, c.id DESC LIMIT 1`,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, notFound("")
	}
	if err != nil {
		return conversation.Conversation{}, wrapStorage("latest conversation", err)
	}
	return value, nil
}

func (r *Repository) Get(ctx context.Context, id conversation.ID) (conversation.Conversation, error) {
	value, err := scanConversation(r.db.QueryRowContext(
		ctx,
		conversationSelect()+` WHERE c.id = ?`,
		string(id),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, notFound(string(id))
	}
	if err != nil {
		return conversation.Conversation{}, wrapStorage("get conversation", err)
	}
	return value, nil
}

func (r *Repository) FindByPrefix(ctx context.Context, prefix string) (conversation.Conversation, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return conversation.Conversation{}, notFound(prefix)
	}
	for _, char := range prefix {
		if (char < '0' || char > '9') &&
			(char < 'a' || char > 'f') &&
			(char < 'A' || char > 'F') &&
			char != '-' {
			return conversation.Conversation{}, notFound(prefix)
		}
	}
	rows, err := r.db.QueryContext(
		ctx,
		conversationSelect()+` WHERE c.id LIKE ? ORDER BY c.updated_at DESC, c.id DESC LIMIT 2`,
		prefix+"%",
	)
	if err != nil {
		return conversation.Conversation{}, wrapStorage("find conversation prefix", err)
	}
	defer rows.Close()
	values := make([]conversation.Conversation, 0, 2)
	for rows.Next() {
		value, err := scanConversation(rows)
		if err != nil {
			return conversation.Conversation{}, wrapStorage("scan conversation prefix", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return conversation.Conversation{}, wrapStorage("iterate conversation prefix", err)
	}
	switch len(values) {
	case 0:
		return conversation.Conversation{}, notFound(prefix)
	case 1:
		return values[0], nil
	default:
		return conversation.Conversation{}, ambiguous(prefix)
	}
}

func (r *Repository) Create(ctx context.Context, value conversation.Conversation) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO conversations(
			id, title, title_source, title_attempted_at, system_prompt_snapshot,
			provider, model, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(value.ID),
		value.Title,
		string(value.TitleSource),
		value.TitleAttemptedAt,
		value.SystemPromptSnapshot,
		value.Provider,
		value.Model,
		value.CreatedAt.Unix(),
		value.UpdatedAt.Unix(),
	)
	if err != nil {
		return wrapStorage("create conversation", err)
	}
	return nil
}

func (r *Repository) SetCurrent(ctx context.Context, id conversation.ID) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO app_state(key, value) VALUES('current_conversation_id', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		string(id),
	)
	if err != nil {
		return wrapStorage("set current conversation", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, limit int) ([]conversation.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(
		ctx,
		conversationSelect()+` ORDER BY c.updated_at DESC, c.id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, wrapStorage("list conversations", err)
	}
	defer rows.Close()
	values := make([]conversation.Conversation, 0)
	for rows.Next() {
		value, err := scanConversation(rows)
		if err != nil {
			return nil, wrapStorage("scan conversation list", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapStorage("iterate conversation list", err)
	}
	return values, nil
}

func (r *Repository) Messages(
	ctx context.Context,
	id conversation.ID,
	limit int,
) ([]conversation.Message, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT
			id, conversation_id, role, content, reasoning_content, status,
			provider, model, input_tokens, output_tokens, reasoning_tokens,
			error, created_at
		FROM (
			SELECT * FROM messages
			WHERE conversation_id = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		)
		ORDER BY created_at ASC, id ASC`,
		string(id),
		limit,
	)
	if err != nil {
		return nil, wrapStorage("list messages", err)
	}
	defer rows.Close()
	values := make([]conversation.Message, 0)
	for rows.Next() {
		value, err := scanMessage(rows)
		if err != nil {
			return nil, wrapStorage("scan message", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapStorage("iterate messages", err)
	}
	return values, nil
}

func (r *Repository) AppendUserAndClaimTitle(
	ctx context.Context,
	message conversation.Message,
	now int64,
) (conversation.AppendResult, error) {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.AppendResult{}, wrapStorage("begin append user message", err)
	}
	defer transaction.Rollback()

	var count int
	if err := transaction.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`,
		string(message.ConversationID),
	).Scan(&count); err != nil {
		return conversation.AppendResult{}, wrapStorage("count conversation messages", err)
	}
	claimed := 0
	if count == 0 {
		result, err := transaction.ExecContext(
			ctx,
			`UPDATE conversations
			 SET title_attempted_at = ?
			 WHERE id = ? AND title_attempted_at = 0`,
			now,
			string(message.ConversationID),
		)
		if err != nil {
			return conversation.AppendResult{}, wrapStorage("claim title generation", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return conversation.AppendResult{}, wrapStorage("read title claim result", err)
		}
		claimed = int(affected)
	}
	if err := insertMessage(ctx, transaction, message); err != nil {
		return conversation.AppendResult{}, err
	}
	if err := touchConversation(ctx, transaction, message); err != nil {
		return conversation.AppendResult{}, err
	}
	value, err := getConversationTx(ctx, transaction, message.ConversationID)
	if err != nil {
		return conversation.AppendResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return conversation.AppendResult{}, wrapStorage("commit user message", err)
	}
	return conversation.AppendResult{
		Conversation: value,
		TitleClaimed: claimed == 1,
	}, nil
}

func (r *Repository) SaveMessage(ctx context.Context, message conversation.Message) error {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return wrapStorage("begin save message", err)
	}
	defer transaction.Rollback()
	if err := insertMessage(ctx, transaction, message); err != nil {
		return err
	}
	if err := touchConversation(ctx, transaction, message); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return wrapStorage("commit message", err)
	}
	return nil
}

func (r *Repository) UpdateTitleIfNotUser(
	ctx context.Context,
	id conversation.ID,
	title string,
	source conversation.TitleSource,
) (bool, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return false, nil
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE conversations
		 SET title = ?, title_source = ?
		 WHERE id = ? AND title_source <> 'user'`,
		title,
		string(source),
		string(id),
	)
	if err != nil {
		return false, wrapStorage("update title", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, wrapStorage("read title update result", err)
	}
	return affected > 0, nil
}

func (r *Repository) Rename(ctx context.Context, id conversation.ID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return failure.New(
			failure.KindUsage,
			"conversation.title_empty",
			map[string]string{"conversation": string(id)},
		)
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE conversations SET title = ?, title_source = 'user' WHERE id = ?`,
		title,
		string(id),
	)
	if err != nil {
		return wrapStorage("rename conversation", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapStorage("read rename result", err)
	}
	if affected == 0 {
		return notFound(string(id))
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id conversation.ID) error {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return wrapStorage("begin delete conversation", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, string(id))
	if err != nil {
		return wrapStorage("delete conversation", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapStorage("read delete conversation result", err)
	}
	if affected == 0 {
		return notFound(string(id))
	}
	current, err := currentIDTx(ctx, transaction)
	if err != nil {
		return err
	}
	if current == string(id) {
		latest, err := latestIDTx(ctx, transaction)
		if err != nil {
			return err
		}
		if latest == "" {
			if _, err := transaction.ExecContext(ctx, `DELETE FROM app_state WHERE key='current_conversation_id'`); err != nil {
				return wrapStorage("clear deleted current conversation", err)
			}
		} else if _, err := transaction.ExecContext(
			ctx,
			`UPDATE app_state SET value = ? WHERE key='current_conversation_id'`,
			latest,
		); err != nil {
			return wrapStorage("replace deleted current conversation", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return wrapStorage("commit delete conversation", err)
	}
	return nil
}

func (r *Repository) AcquireLease(ctx context.Context, lease conversation.Lease) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO conversation_leases(conversation_id, lease_owner, lease_expires_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
		   lease_owner = excluded.lease_owner,
		   lease_expires_at = excluded.lease_expires_at`,
		string(lease.ConversationID),
		lease.Owner,
		lease.ExpiresAt.Unix(),
	)
	if err != nil {
		return wrapStorage("acquire conversation lease", err)
	}
	return nil
}

func (r *Repository) ReleaseLease(ctx context.Context, id conversation.ID, owner string) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM conversation_leases WHERE conversation_id = ? AND lease_owner = ?`,
		string(id),
		owner,
	)
	if err != nil {
		return wrapStorage("release conversation lease", err)
	}
	return nil
}

func (r *Repository) Cleanup(
	ctx context.Context,
	plan conversation.RetentionPlan,
) (conversation.RetentionResult, error) {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.RetentionResult{}, wrapStorage("begin retention cleanup", err)
	}
	defer transaction.Rollback()

	current, err := currentIDTx(ctx, transaction)
	if err != nil {
		return conversation.RetentionResult{}, err
	}
	if current != "" {
		var exists int
		if err := transaction.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM conversations WHERE id = ?`,
			current,
		).Scan(&exists); err != nil {
			return conversation.RetentionResult{}, wrapStorage("check current conversation", err)
		}
		if exists == 0 {
			if _, err := transaction.ExecContext(
				ctx,
				`DELETE FROM app_state WHERE key = 'current_conversation_id'`,
			); err != nil {
				return conversation.RetentionResult{}, wrapStorage("clear invalid current conversation", err)
			}
			current = ""
		}
	}
	latest, err := latestIDTx(ctx, transaction)
	if err != nil {
		return conversation.RetentionResult{}, err
	}
	limit := plan.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := transaction.QueryContext(
		ctx,
		`SELECT c.id
		 FROM conversations c
		 WHERE c.updated_at < ?
		   AND (? = '' OR c.id <> ?)
		   AND (? = '' OR c.id <> ?)
		   AND NOT EXISTS (
		     SELECT 1 FROM conversation_leases l
		     WHERE l.conversation_id = c.id AND l.lease_expires_at >= ?
		   )
		 ORDER BY c.updated_at ASC
		 LIMIT ?`,
		plan.Cutoff.Unix(),
		current,
		current,
		latest,
		latest,
		time.Now().Unix(),
		limit,
	)
	if err != nil {
		return conversation.RetentionResult{}, wrapStorage("select retention candidates", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return conversation.RetentionResult{}, wrapStorage("scan retention candidate", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return conversation.RetentionResult{}, wrapStorage("close retention candidates", err)
	}
	deleted := 0
	for _, id := range ids {
		result, err := transaction.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
		if err != nil {
			return conversation.RetentionResult{}, wrapStorage("delete retained conversation", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return conversation.RetentionResult{}, wrapStorage("read retention delete result", err)
		}
		deleted += int(affected)
	}
	if err := transaction.Commit(); err != nil {
		return conversation.RetentionResult{}, wrapStorage("commit retention cleanup", err)
	}
	return conversation.RetentionResult{Deleted: deleted}, nil
}

func (r *Repository) LastCleanupAt(ctx context.Context) (int64, error) {
	var value string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT value FROM app_state WHERE key = 'last_retention_cleanup_at'`,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrapStorage("read last cleanup", err)
	}
	var unix int64
	_, err = fmt.Sscan(value, &unix)
	if err != nil {
		return 0, wrapStorage("parse last cleanup", err)
	}
	return unix, nil
}

func (r *Repository) MarkCleanupAt(ctx context.Context, unix int64) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO app_state(key, value) VALUES('last_retention_cleanup_at', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		fmt.Sprintf("%d", unix),
	)
	if err != nil {
		return wrapStorage("mark cleanup", err)
	}
	return nil
}

func (r *Repository) AddRecentRoute(ctx context.Context, route provider.Route, limit int) error {
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	routes, err := r.RecentRoutes(ctx, 10)
	if err != nil {
		return err
	}
	filtered := make([]provider.Route, 0, len(routes)+1)
	filtered = append(filtered, route)
	for _, existing := range routes {
		if existing.Provider == route.Provider && existing.Model == route.Model {
			continue
		}
		filtered = append(filtered, existing)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return wrapStorage("encode recent routes", err)
	}
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO app_state(key, value) VALUES('recent_model_routes', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		string(encoded),
	); err != nil {
		return wrapStorage("save recent routes", err)
	}
	return nil
}

func (r *Repository) RecentRoutes(ctx context.Context, limit int) ([]provider.Route, error) {
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	var value string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT value FROM app_state WHERE key = 'recent_model_routes'`,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return []provider.Route{}, nil
	}
	if err != nil {
		return nil, wrapStorage("read recent routes", err)
	}
	var routes []provider.Route
	if err := json.Unmarshal([]byte(value), &routes); err != nil {
		return []provider.Route{}, nil
	}
	if len(routes) > limit {
		routes = routes[:limit]
	}
	return routes, nil
}

func (r *Repository) ModelCapability(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT value FROM app_state WHERE key = ?`,
		key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, wrapStorage("read model capability", err)
	}
	return value, true, nil
}

func (r *Repository) SaveModelCapability(ctx context.Context, key string, value string) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO app_state(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key,
		value,
	)
	if err != nil {
		return wrapStorage("save model capability", err)
	}
	return nil
}

func getConversationTx(
	ctx context.Context,
	transaction *sql.Tx,
	id conversation.ID,
) (conversation.Conversation, error) {
	value, err := scanConversation(transaction.QueryRowContext(
		ctx,
		conversationSelect()+` WHERE c.id = ?`,
		string(id),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, notFound(string(id))
	}
	if err != nil {
		return conversation.Conversation{}, wrapStorage("get conversation transaction", err)
	}
	return value, nil
}

func insertMessage(ctx context.Context, transaction *sql.Tx, message conversation.Message) error {
	_, err := transaction.ExecContext(
		ctx,
		`INSERT INTO messages(
			id, conversation_id, role, content, reasoning_content, status,
			provider, model, input_tokens, output_tokens, reasoning_tokens,
			error, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ID,
		string(message.ConversationID),
		message.Role,
		message.Content,
		message.ReasoningContent,
		string(message.Status),
		message.Provider,
		message.Model,
		message.InputTokens,
		message.OutputTokens,
		message.ReasoningTokens,
		message.Error,
		message.CreatedAt.Unix(),
	)
	if err != nil {
		return wrapStorage("insert message", err)
	}
	return nil
}

func touchConversation(ctx context.Context, transaction *sql.Tx, message conversation.Message) error {
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE conversations
		 SET updated_at = ?, provider = ?, model = ?
		 WHERE id = ?`,
		message.CreatedAt.Unix(),
		message.Provider,
		message.Model,
		string(message.ConversationID),
	)
	if err != nil {
		return wrapStorage("touch conversation", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapStorage("read conversation update result", err)
	}
	if affected == 0 {
		return notFound(string(message.ConversationID))
	}
	return nil
}

func currentIDTx(ctx context.Context, transaction *sql.Tx) (string, error) {
	var current string
	err := transaction.QueryRowContext(
		ctx,
		`SELECT value FROM app_state WHERE key = 'current_conversation_id'`,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", wrapStorage("read current conversation transaction", err)
	}
	return current, nil
}

func latestIDTx(ctx context.Context, transaction *sql.Tx) (string, error) {
	var latest string
	err := transaction.QueryRowContext(
		ctx,
		`SELECT id FROM conversations ORDER BY updated_at DESC, id DESC LIMIT 1`,
	).Scan(&latest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", wrapStorage("read latest conversation transaction", err)
	}
	return latest, nil
}
