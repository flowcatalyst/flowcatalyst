package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCredentialPublishRefusesABadSetup(t *testing.T) {
	t.Setenv("FC_CREDENTIAL_PUBLISH", "")
	assert.NoError(t, LoadEnv().CheckCredentialPublish())
	assert.False(t, LoadEnv().CredentialPublish.Enabled())

	t.Setenv("FC_CREDENTIAL_PUBLISH", "aws-ps") // no prefix
	assert.Error(t, LoadEnv().CheckCredentialPublish())

	t.Setenv("FC_CREDENTIAL_PUBLISH_PREFIX", "/flowcatalyst/np/")
	cfg := LoadEnv()
	require.NoError(t, cfg.CheckCredentialPublish())
	assert.True(t, cfg.CredentialPublish.Enabled())
}
