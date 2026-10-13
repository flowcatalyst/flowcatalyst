package operations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// The domain events carry Java's and Rust's code, envelope and data
// (docs/platform-bug-fixes.md F11): {domainId, hostname, owner}, owner being
// "platform" or the owning client's id, on release too.
func TestDomainEvents_SharedWithJavaAndRust(t *testing.T) {
	client := "clt_0000000000001"
	cases := []struct {
		name  string
		event usecase.DomainEvent
		code  string
		data  string
	}{
		{
			"claimed by a client",
			operations.DomainClaimed{DomainID: "fnd_0000000000001", Hostname: "acme.com", Owner: operations.OwnerOf(&client)},
			"platform:function:domain:claimed", `{"domainId":"fnd_0000000000001","hostname":"acme.com","owner":"clt_0000000000001"}`,
		},
		{
			"claimed by the platform",
			operations.DomainClaimed{DomainID: "fnd_0000000000001", Hostname: "acme.com", Owner: operations.OwnerOf(nil)},
			"platform:function:domain:claimed", `{"domainId":"fnd_0000000000001","hostname":"acme.com","owner":"platform"}`,
		},
		{
			"released",
			operations.DomainReleased{DomainID: "fnd_0000000000001", Hostname: "acme.com", Owner: operations.OwnerOf(&client)},
			"platform:function:domain:released", `{"domainId":"fnd_0000000000001","hostname":"acme.com","owner":"clt_0000000000001"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.code, c.event.EventType())
			assert.Equal(t, "platform:function", c.event.Source())
			assert.Equal(t, "platform.function-domain.fnd_0000000000001", c.event.Subject())
			assert.Equal(t, "platform:function-domain:fnd_0000000000001", c.event.MessageGroup())
			data, err := c.event.ToDataJSON()
			require.NoError(t, err)
			assert.JSONEq(t, c.data, string(data))
		})
	}
}
