package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/KevinLeeNJ/ask/internal/domain/conversation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/platform/configdir"
)

//go:embed migrations/001_initial.sql
var initialMigration string

const schemaVersion = "1"

type Repository struct {
	db *sql.DB
}

func DefaultPath() (string, error) {
	configDir, err := configdir.AskDir()
	if err != nil {
		return "", failure.Wrap(failure.KindConfig, "config.user_dir_unavailable", nil, err)
	}
	return filepath.Join(configDir, "ask.db"), nil
}

func Open(path string) (*Repository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, failure.New(failure.KindConfig, "storage.path_missing", nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, failure.Wrap(
			failure.KindConfig,
			"storage.open_failed",
			map[string]string{"path": path, "reason": err.Error()},
			err,
		)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, failure.Wrap(
			failure.KindConfig,
			"storage.open_failed",
			map[string]string{"path": path, "reason": err.Error()},
			err,
		)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	repository := &Repository{db: db}
	if err := repository.initialize(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		db.Close()
		return nil, failure.Wrap(
			failure.KindConfig,
			"storage.permission_failed",
			map[string]string{"path": path, "reason": err.Error()},
			err,
		)
	}
	return repository, nil
}

func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Repository) initialize(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return failure.Wrap(
				failure.KindConfig,
				"storage.initialize_failed",
				map[string]string{"statement": statement, "reason": err.Error()},
				err,
			)
		}
	}

	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return failure.Wrap(failure.KindConfig, "storage.migration_failed", nil, err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, initialMigration); err != nil {
		return failure.Wrap(failure.KindConfig, "storage.migration_failed", nil, err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO app_state(key, value) VALUES('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		schemaVersion,
	); err != nil {
		return failure.Wrap(failure.KindConfig, "storage.migration_failed", nil, err)
	}
	if err := transaction.Commit(); err != nil {
		return failure.Wrap(failure.KindConfig, "storage.migration_failed", nil, err)
	}
	return nil
}

func scanConversation(scanner interface {
	Scan(dest ...any) error
}) (conversation.Conversation, error) {
	var (
		value          conversation.Conversation
		id             string
		titleSource    string
		titleAttempted int64
		createdAt      int64
		updatedAt      int64
		messageCount   int
	)
	if err := scanner.Scan(
		&id,
		&value.Title,
		&titleSource,
		&titleAttempted,
		&value.SystemPromptSnapshot,
		&value.Provider,
		&value.Model,
		&createdAt,
		&updatedAt,
		&messageCount,
	); err != nil {
		return conversation.Conversation{}, err
	}
	value.ID = conversation.ID(id)
	value.TitleSource = conversation.TitleSource(titleSource)
	value.TitleAttemptedAt = titleAttempted
	value.CreatedAt = unixTime(createdAt)
	value.UpdatedAt = unixTime(updatedAt)
	value.MessageCount = messageCount
	return value, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (conversation.Message, error) {
	var (
		value           conversation.Message
		id              string
		conversationID  string
		status          string
		inputTokens     sql.NullInt64
		outputTokens    sql.NullInt64
		reasoningTokens sql.NullInt64
		errorText       sql.NullString
		createdAt       int64
	)
	if err := scanner.Scan(
		&id,
		&conversationID,
		&value.Role,
		&value.Content,
		&value.ReasoningContent,
		&status,
		&value.Provider,
		&value.Model,
		&inputTokens,
		&outputTokens,
		&reasoningTokens,
		&errorText,
		&createdAt,
	); err != nil {
		return conversation.Message{}, err
	}
	value.ID = id
	value.ConversationID = conversation.ID(conversationID)
	value.Status = conversation.MessageStatus(status)
	value.InputTokens = int(inputTokens.Int64)
	value.OutputTokens = int(outputTokens.Int64)
	value.ReasoningTokens = int(reasoningTokens.Int64)
	value.Error = errorText.String
	value.CreatedAt = unixTime(createdAt)
	return value, nil
}

func conversationSelect() string {
	return `SELECT
		c.id,
		c.title,
		c.title_source,
		c.title_attempted_at,
		c.system_prompt_snapshot,
		c.provider,
		c.model,
		c.created_at,
		c.updated_at,
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id)
	FROM conversations c`
}

func wrapStorage(operation string, err error) error {
	if err == nil {
		return nil
	}
	return failure.Wrap(
		failure.KindInternal,
		"storage.operation_failed",
		map[string]string{"operation": operation, "reason": err.Error()},
		err,
	)
}

func notFound(id string) error {
	return failure.New(
		failure.KindUsage,
		"conversation.not_found",
		map[string]string{"conversation": id},
	)
}

func ambiguous(prefix string) error {
	return failure.New(
		failure.KindUsage,
		"conversation.prefix_ambiguous",
		map[string]string{"prefix": prefix},
	)
}

func unixTime(value int64) time.Time {
	return time.Unix(value, 0)
}
