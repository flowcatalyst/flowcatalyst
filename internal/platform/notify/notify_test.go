package notify

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
)

type captureMailer struct{ msgs []email.Message }

func (c *captureMailer) Send(_ context.Context, m email.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

func TestNewTrustedDevice_EscapesLabel(t *testing.T) {
	m := &captureMailer{}
	n := New(m)
	n.NewTrustedDevice(context.Background(), "u@example.com", `<a href="https://evil.example">click</a><!-- x`)
	require.Len(t, m.msgs, 1)
	body := m.msgs[0].HTMLBody
	assert.NotContains(t, body, "<a href")
	assert.NotContains(t, body, "<!--")
	assert.Contains(t, body, "&lt;a href=&#34;https://evil.example&#34;&gt;click&lt;/a&gt;&lt;!-- x")
}

func TestNewTrustedDevice_PlainLabelUnchanged(t *testing.T) {
	m := &captureMailer{}
	New(m).NewTrustedDevice(context.Background(), "u@example.com", "Chrome on macOS")
	require.Len(t, m.msgs, 1)
	assert.Contains(t, m.msgs[0].HTMLBody, `<p style="color:#555">Chrome on macOS</p>`)
}

func TestOtherBodies_EscapeInterpolatedValues(t *testing.T) {
	m := &captureMailer{}
	n := New(m).WithName(func(context.Context) string { return "<b>Brand</b>" })
	n.PasswordChanged(context.Background(), "u@example.com")
	n.TwoFactorEnrolled(context.Background(), "u@example.com", "<i>x</i>")
	n.ResetApprovalNeeded(context.Background(), "u@example.com", `https://h/x"><script>`)
	require.Len(t, m.msgs, 3)
	for _, msg := range m.msgs {
		assert.NotContains(t, msg.HTMLBody, "<b>")
		assert.NotContains(t, msg.HTMLBody, "<i>")
		assert.NotContains(t, msg.HTMLBody, "<script>")
	}
}
