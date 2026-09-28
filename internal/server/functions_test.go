package server

import (
	"net/netip"
	"testing"
)

func TestLoadFunctionRunnerEnv(t *testing.T) {
	t.Setenv("FC_FUNCTIONS_ENABLED", "true")
	t.Setenv("FC_FUNCTIONS_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.7, nonsense")
	c := LoadFunctionRunnerEnv(EnvCfg{PlatformEnabled: true, APIPort: 8080})
	if !c.Enabled || c.Port != DefaultFunctionRunnerPort || c.Pool != "default" || c.PlatformURL != "http://127.0.0.1:8080" {
		t.Fatalf("config = %+v", c)
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.168.1.7/32")}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[0] != want[0] || c.TrustedProxies[1] != want[1] {
		t.Fatalf("trusted proxies = %v", c.TrustedProxies)
	}
	if c.SetMemoryLimit {
		t.Error("GOMEMLIMIT set while the platform shares the process")
	}
	if c := LoadFunctionRunnerEnv(EnvCfg{}); !c.SetMemoryLimit || c.PlatformURL != "" {
		t.Errorf("a runner-only process: %+v", c)
	}
}
