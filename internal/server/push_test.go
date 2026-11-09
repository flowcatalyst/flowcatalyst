package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPushEnvironment(t *testing.T) {
	got := PushEnvironment([]string{
		"FC_PUSH_ENABLED=true", "FC_PUSH_STREAM_SECRET=s3cret", "FC_PUSH_DRIVER=redis", "FC_PUSH_STREAM_MAX_LIFETIME=30m",
		"FC_ROUTER_ENABLED=true", "PUSH_DRIVER=ignored", "APP_ENV=local", "malformed",
	})
	assert.Equal(t, map[string]string{
		"APP_ENV": "production", "PUSH_STREAM_SECRET": "s3cret", "PUSH_DRIVER": "redis", "PUSH_STREAM_MAX_LIFETIME": "30m",
	}, got)
}

func TestPushEnabledFlag(t *testing.T) {
	t.Setenv("FC_PUSH_ENABLED", "")
	assert.False(t, LoadEnv().PushEnabled, "off by default in a deployed environment")
	t.Setenv("FC_PUSH_ENABLED", "true")
	assert.True(t, LoadEnv().PushEnabled)
}
