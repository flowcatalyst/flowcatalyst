package fn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// auth is an endpoint's declared authentication mode (plan §5.4).
type auth string

const (
	authWebhook  auth = "webhook"
	authPlatform auth = "platform"
	authNone     auth = "none"
)

type endpointDef struct {
	Method  string
	Path    string
	Auth    auth
	Options endpointOptions
}

type subDef struct {
	EventType      string
	Path           string
	Mode           string
	MaxRetries     *int
	DataOnly       bool
	TimeoutSeconds *int
}

type schedDef struct {
	Cron     string
	Timezone string
	Path     string
	Payload  json.RawMessage
}

// registry accumulates every init()-time declaration (endpoints,
// subscriptions, schedules, config/secret/db keys, http allowlist, emits)
// into both the in-guest http.ServeMux used by dispatch and the data used to
// build the describe document (plan §5.4).
type registry struct {
	mu sync.Mutex

	mux       *http.ServeMux
	endpoints []endpointDef
	subs      []subDef
	scheds    []schedDef
	config    []string
	secret    []string
	db        []string
	httpAllow []string
	emits     []string
}

func newRegistry() *registry {
	return &registry{mux: http.NewServeMux()}
}

// reg is the package-level registry populated by init() calls in guest
// code, per the declarative style in plan §9.
var reg = newRegistry()

// splitPattern parses a "METHOD /path" registration pattern, the same
// syntax Go 1.22's http.ServeMux accepts for a method-qualified pattern.
// Every fn.Webhook/Platform/Open registration must name a method explicitly
// (a deliberate SDK choice, tighter than bare ServeMux patterns which allow
// a method-less pattern): declared endpoints always report a concrete
// "method" in describe, and Webhook additionally enforces POST.
func splitPattern(pattern string) (method, path string) {
	i := strings.IndexByte(pattern, ' ')
	if i < 0 {
		panic(fmt.Sprintf("fn: endpoint pattern must be \"METHOD /path\", got %q", pattern))
	}
	method = pattern[:i]
	path = strings.TrimSpace(pattern[i+1:])
	if method == "" || path == "" {
		panic(fmt.Sprintf("fn: endpoint pattern must be \"METHOD /path\", got %q", pattern))
	}
	return method, path
}

func registerEndpoint(a auth, pattern string, h http.HandlerFunc, opts []EndpointOption) {
	method, path := splitPattern(pattern)
	if a == authWebhook && method != http.MethodPost {
		panic(fmt.Sprintf("fn: webhook endpoint %q must be POST-only, got method %q", pattern, method))
	}
	if h == nil {
		panic(fmt.Sprintf("fn: endpoint %q registered with a nil handler", pattern))
	}

	var cfg endpointOptions
	for _, o := range opts {
		o(&cfg)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.mux.HandleFunc(pattern, h)
	reg.endpoints = append(reg.endpoints, endpointDef{Method: method, Path: path, Auth: a, Options: cfg})
}
