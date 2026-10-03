package router

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func TestLazyMessageLoggerCarriesCorrelationAttrs(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(old)

	qm := common.QueuedMessage{Message: common.Message{ID: "m-1"}, QueueIdentifier: "q-1"}
	l := messageLogger("POOL", qm)
	l.Debug("filtered")
	l.With("extra", 1).Warn("hello")

	out := buf.String()
	if strings.Contains(out, "filtered") {
		t.Fatalf("debug line should be filtered: %s", out)
	}
	for _, want := range []string{"message_id=m-1", "pool=POOL", "queue=q-1", "extra=1", "hello"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}
