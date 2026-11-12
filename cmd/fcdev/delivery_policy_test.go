package main

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// Developers keep their loopback webhook receivers: with fcdev's relaxations
// applied, a loopback customer URL still validates under the customer policy,
// and the internal policy follows.
func TestDevEnvCfg_RelaxesBothDeliveryPolicies(t *testing.T) {
	t.Setenv("FC_DELIVERY_ALLOW_LOOPBACK", "")
	t.Setenv("FC_DELIVERY_ALLOW_PRIVATE", "")
	defD, defI := *netguard.Default, *netguard.Internal
	t.Cleanup(func() { *netguard.Default, *netguard.Internal = defD, defI })

	devEnvCfg(startOpts{APIPort: 18181, MetricsPort: 9090}, "postgres://x", routerCredentials{})

	for name, p := range map[string]*netguard.Policy{"customer": netguard.Default, "internal": netguard.Internal} {
		if err := p.ValidateURL("http://127.0.0.1:9999/hook"); err != nil {
			t.Errorf("%s policy refuses a loopback URL under fcdev: %v", name, err)
		}
		if err := p.ValidateURL("http://10.1.2.3/hook"); err != nil {
			t.Errorf("%s policy refuses a private URL under fcdev: %v", name, err)
		}
		if err := p.ValidateURL("http://169.254.169.254/"); err == nil {
			t.Errorf("%s policy allows the metadata address", name)
		}
	}
}
