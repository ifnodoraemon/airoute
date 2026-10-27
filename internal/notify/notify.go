package notify

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Level classifies an alert. It maps to the message prefix/tone only; routing
// (which channels receive which level) is the caller's decision.
type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

// Message is one notification, independent of the transport. Budget events use
// it directly; keeping the model transport-neutral is what lets the same alert
// go to e-mail, a webhook and an IM bot without three payload builders at the
// call site.
type Message struct {
	Event     string
	Level     Level
	Title     string
	Body      string
	Fields    map[string]string
	Timestamp time.Time
}

// Normalize fills defaults and sorts nothing (callers may rely on map order for
// machine endpoints); the human renderers sort keys themselves.
func (m Message) Normalize() Message {
	if m.Level == "" {
		m.Level = LevelInfo
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	if m.Title == "" {
		m.Title = "Airoute notification"
	}
	return m
}

// FieldsText renders key: value lines in sorted order for deterministic
// messages and tests.
func (m Message) FieldsText() string {
	if len(m.Fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m.Fields))
	for key := range m.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", key, m.Fields[key]))
	}
	return strings.Join(lines, "\n")
}

// Sender delivers one message to one transport.
type Sender interface {
	Name() string
	Send(ctx context.Context, msg Message) error
}

// MultiSender fans a message out to every configured transport and joins the
// failures. One broken channel must not prevent the others from notifying.
type MultiSender struct {
	Senders []Sender
}

// Send implements Sender.
func (m MultiSender) Send(ctx context.Context, msg Message) error {
	var errs []error
	for _, sender := range m.Senders {
		if sender == nil {
			continue
		}
		if err := sender.Send(ctx, msg); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sender.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// Name identifies the composite sender in logs.
func (m MultiSender) Name() string { return "multi" }

// Configured reports whether at least one transport is active.
func (m MultiSender) Configured() bool { return len(m.Senders) > 0 }
