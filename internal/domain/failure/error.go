package failure

import (
	"errors"
	"fmt"
)

// Kind is a stable error category used by presentation and exit-code mapping.
type Kind string

const (
	KindUsage     Kind = "usage"
	KindConfig    Kind = "config"
	KindUpstream  Kind = "upstream"
	KindTransport Kind = "transport"
	KindProtocol  Kind = "protocol"
	KindCanceled  Kind = "canceled"
	KindInternal  Kind = "internal"
)

// Error carries a stable message identifier and structured parameters.
// User-visible text is produced by the i18n layer, not by business packages.
type Error struct {
	Kind      Kind
	MessageID string
	Args      map[string]string
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.MessageID == "" {
		return string(e.Kind)
	}
	if e.Cause == nil {
		return e.MessageID
	}
	return fmt.Sprintf("%s: %v", e.MessageID, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(kind Kind, messageID string, args map[string]string) *Error {
	return &Error{Kind: kind, MessageID: messageID, Args: args}
}

func Wrap(kind Kind, messageID string, args map[string]string, cause error) *Error {
	return &Error{Kind: kind, MessageID: messageID, Args: args, Cause: cause}
}

func KindOf(err error) Kind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindInternal
}

func Info(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return &Error{Kind: KindInternal, MessageID: "error.internal", Cause: err}
}
