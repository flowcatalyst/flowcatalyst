package router

import "github.com/flowcatalyst/flowcatalyst-go/internal/netguard"

// These tests deliver to httptest servers, which listen on loopback; the
// production policy refuses that on purpose (see internal/netguard).
func init() { netguard.Default.AllowLoopback = true }
