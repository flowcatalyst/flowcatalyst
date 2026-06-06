// Package email sends transactional emails (password reset, …) over SMTP.
//
// Port of the Rust shared/email_service.rs: configured from SMTP_* env vars
// (FC_-prefixed names take precedence over the bare TS-style names). With
// SMTP_SECURE=false (the default, and the SendGrid :587 setup) it uses
// STARTTLS; SMTP_SECURE=true uses implicit TLS (:465). When SMTP_HOST isn't
// set it returns a LogService that logs the message instead of sending, so the
// platform still boots without a mailer (1:1 with Rust create_email_service).
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"os"
	"strings"
)

// Message is a single HTML email.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
}

// Service sends an email.
type Service interface {
	Send(ctx context.Context, m Message) error
}

// LogService logs the email instead of sending it — used when SMTP isn't
// configured. Diverges from Rust's LogEmailService by also logging the body:
// with no mailer, set-password links and 2FA email PINs live in the body, so
// dumping it to the console is what makes those flows testable locally.
type LogService struct{}

// Send logs the would-be email (including the body, so dev can read the
// link/PIN) and returns nil.
func (LogService) Send(_ context.Context, m Message) error {
	slog.Warn("[email] SMTP not configured — logging instead of sending",
		"to", m.To, "subject", m.Subject, "body", m.HTMLBody)
	return nil
}

// SMTPService sends real email. secure=true → implicit TLS (e.g. :465);
// secure=false → STARTTLS (e.g. :587, the SendGrid default).
type SMTPService struct {
	host, port, username, password, from string
	secure                               bool
}

// FromEnv builds the email service from the environment. Returns a LogService
// (never nil) when SMTP_HOST isn't set — mirroring Rust create_email_service.
func FromEnv() Service {
	host := envFirst("FC_SMTP_HOST", "SMTP_HOST")
	if host == "" {
		slog.Warn("SMTP not configured (no SMTP_HOST); password-reset and other emails will be logged only")
		return LogService{}
	}
	svc := &SMTPService{
		host:     host,
		port:     orDefault(envFirst("FC_SMTP_PORT", "SMTP_PORT"), "587"),
		username: envFirst("FC_SMTP_USERNAME", "SMTP_USERNAME"),
		password: envFirst("FC_SMTP_PASSWORD", "SMTP_PASSWORD"),
		from:     orDefault(envFirst("FC_SMTP_FROM", "SMTP_FROM"), "noreply@flowcatalyst.local"),
		secure:   envBool("FC_SMTP_SECURE", "SMTP_SECURE"),
	}
	slog.Info("SMTP email service configured", "host", svc.host, "port", svc.port, "from", svc.from, "secure", svc.secure)
	return svc
}

// Send delivers m over SMTP.
func (s *SMTPService) Send(_ context.Context, m Message) error {
	raw := buildMIME(s.from, m.To, m.Subject, m.HTMLBody)
	addr := net.JoinHostPort(s.host, s.port)
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	if s.secure {
		return s.sendImplicitTLS(addr, auth, m.To, raw)
	}
	// STARTTLS path: net/smtp upgrades to TLS when the server advertises it
	// (SendGrid :587 does), then AUTHs and sends.
	if err := smtp.SendMail(addr, auth, s.from, []string{m.To}, raw); err != nil {
		return fmt.Errorf("smtp send (starttls) to %s: %w", addr, err)
	}
	return nil
}

// sendImplicitTLS handles secure=true (TLS from the first byte, e.g. :465),
// which net/smtp.SendMail does not set up for us.
func (s *SMTPService) sendImplicitTLS(addr string, auth smtp.Auth, to string, raw []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
	if err != nil {
		return fmt.Errorf("smtp tls dial %s: %w", addr, err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = c.Close() }()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := wc.Write(raw); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	return c.Quit()
}

// buildMIME assembles a minimal RFC 5322 HTML message with CRLF line endings.
func buildMIME(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func envBool(keys ...string) bool {
	switch strings.ToLower(envFirst(keys...)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
