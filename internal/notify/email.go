package notify

import (
	"context"
	"fmt"
	"mime"
	"net/smtp"
	"strings"
)

// EmailSender delivers alerts over SMTP. It mirrors the existing verification
// mail configuration (SMTP_HOST/PORT) so an operator only configures the relay
// once.
type EmailSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

func (s EmailSender) Name() string { return "email" }

// Validate rejects an incomplete configuration instead of silently skipping
// alerts: a budget notification that never arrives is worse than a startup
// error.
func (s EmailSender) Validate() error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("SMTP host is required")
	}
	if len(s.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if strings.TrimSpace(s.From) == "" && strings.TrimSpace(s.Username) == "" {
		return fmt.Errorf("SMTP from address is required")
	}
	return nil
}

func (s EmailSender) Send(ctx context.Context, msg Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	msg = msg.Normalize()
	from := strings.TrimSpace(s.From)
	if from == "" {
		from = s.Username
	}
	port := s.Port
	if port <= 0 {
		port = 587
	}
	var auth smtp.Auth
	if s.Username != "" || s.Password != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	message := buildEmail(from, s.To, msg)
	if err := smtp.SendMail(fmt.Sprintf("%s:%d", s.Host, port), auth, from, s.To, message); err != nil {
		return fmt.Errorf("send email notification: %w", err)
	}
	return nil
}

// buildEmail renders a deterministic RFC 5322 message. The subject is Q-encoded
// so Chinese titles survive strict mail relays.
func buildEmail(from string, to []string, msg Message) []byte {
	subject := fmt.Sprintf("[Airoute][%s] %s", msg.Level, msg.Title)
	body := plainText(msg)
	var builder strings.Builder
	builder.WriteString("From: " + from + "\r\n")
	builder.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(body)
	builder.WriteString("\r\n")
	return []byte(builder.String())
}
