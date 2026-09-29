package router

import "github.com/flowcatalyst/flowcatalyst-go/internal/ids"

func mint(s string) ids.ClientID {
	return ids.ClientID(s) // want `ids.ClientID conversion outside the platform layers`
}

func ok(id ids.ClientID) string { return string(id) }
