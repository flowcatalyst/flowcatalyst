package email

import (
	"context"
	"strings"
	"testing"
)

func TestFromEnvLogServiceWhenUnconfigured(t *testing.T) {
	t.Setenv("FC_SMTP_HOST", "")
	t.Setenv("SMTP_HOST", "")
	svc := FromEnv()
	if _, ok := svc.(LogService); !ok {
		t.Fatalf("expected LogService when SMTP_HOST unset, got %T", svc)
	}
	if err := svc.Send(context.Background(), Message{To: "a@b.com", Subject: "x", HTMLBody: "<p>hi</p>"}); err != nil {
		t.Fatalf("LogService.Send must not error: %v", err)
	}
}

func TestFromEnvSMTPService(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.sendgrid.net")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "apikey")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "mailer@inhancesc.com")
	t.Setenv("SMTP_SECURE", "false")
	svc, ok := FromEnv().(*SMTPService)
	if !ok {
		t.Fatalf("expected *SMTPService, got %T", FromEnv())
	}
	if svc.host != "smtp.sendgrid.net" || svc.port != "587" || svc.from != "mailer@inhancesc.com" || svc.secure {
		t.Fatalf("unexpected SMTP config: %+v", *svc)
	}
}

func TestFCPrefixWins(t *testing.T) {
	t.Setenv("SMTP_HOST", "ts.example.com")
	t.Setenv("FC_SMTP_HOST", "fc.example.com")
	svc, ok := FromEnv().(*SMTPService)
	if !ok {
		t.Fatalf("expected *SMTPService, got %T", FromEnv())
	}
	if svc.host != "fc.example.com" {
		t.Fatalf("FC_ prefix should win: got %q", svc.host)
	}
}

func TestBuildMIME(t *testing.T) {
	raw := string(buildMIME("from@x.com", "to@y.com", "Reset your password", "<p>hi</p>"))
	for _, want := range []string{
		"From: from@x.com\r\n",
		"To: to@y.com\r\n",
		"Subject: Reset your password\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/html; charset=UTF-8\r\n",
		"\r\n<p>hi</p>",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("MIME missing %q in:\n%s", want, raw)
		}
	}
}
