package server

import (
	"strings"
	"testing"
)

func TestCheckMCPListen(t *testing.T) {
	cases := []struct {
		name    string
		cfg     EnvCfg
		wantErr bool
	}{
		{"disabled: nothing to check", EnvCfg{MCPEnabled: false, MCPBind: "0.0.0.0", MCPPort: 8090}, false},
		{"loopback, no token: as today", EnvCfg{MCPEnabled: true, MCPBind: "127.0.0.1", MCPPort: 8090}, false},
		{"all interfaces, no token: refused", EnvCfg{MCPEnabled: true, MCPBind: "0.0.0.0", MCPPort: 8090}, true},
		{"empty bind, no token: refused", EnvCfg{MCPEnabled: true, MCPBind: "", MCPPort: 8090}, true},
		{"ipv6 any, no token: refused", EnvCfg{MCPEnabled: true, MCPBind: "::", MCPPort: 8090}, true},
		{"all interfaces with a token: allowed", EnvCfg{MCPEnabled: true, MCPBind: "0.0.0.0", MCPPort: 8090, MCPAuthToken: "t"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.CheckMCPListen()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "FC_MCP_AUTH_TOKEN") {
				t.Fatalf("error must name the variable: %v", err)
			}
		})
	}
}

func TestLoadEnvReadsMCPAuthToken(t *testing.T) {
	t.Setenv("FC_MCP_AUTH_TOKEN", "abc")
	if got := LoadEnv().MCPAuthToken; got != "abc" {
		t.Fatalf("MCPAuthToken = %q", got)
	}
}
