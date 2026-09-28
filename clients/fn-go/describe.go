package fn

import "encoding/json"

// Describe document shapes (plan §5.4). Field order here drives JSON key
// order for readability only; JSON object key order is not semantically
// significant on the wire.

type describeDoc struct {
	ABI           int            `json:"abi"`
	Endpoints     []endpointJSON `json:"endpoints"`
	Subscriptions []subJSON      `json:"subscriptions,omitempty"`
	Schedules     []schedJSON    `json:"schedules,omitempty"`
	Config        []string       `json:"config,omitempty"`
	Secrets       []string       `json:"secrets,omitempty"`
	DB            []string       `json:"db,omitempty"`
	HTTPAllow     []string       `json:"httpAllow,omitempty"`
	Emits         []string       `json:"emits,omitempty"`
}

type corsJSON struct {
	Origins          []string `json:"origins,omitempty"`
	Methods          []string `json:"methods,omitempty"`
	Headers          []string `json:"headers,omitempty"`
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
}

type endpointJSON struct {
	Method       string    `json:"method,omitempty"`
	Path         string    `json:"path"`
	Auth         string    `json:"auth"`
	MaxBodyBytes *int64    `json:"maxBodyBytes,omitempty"`
	TimeoutMs    *int64    `json:"timeoutMs,omitempty"`
	CORS         *corsJSON `json:"cors,omitempty"`
}

type subJSON struct {
	EventType      string `json:"eventType"`
	Path           string `json:"path"`
	Mode           string `json:"mode,omitempty"`
	MaxRetries     *int   `json:"maxRetries,omitempty"`
	DataOnly       bool   `json:"dataOnly,omitempty"`
	TimeoutSeconds *int   `json:"timeoutSeconds,omitempty"`
}

type schedJSON struct {
	Cron     string          `json:"cron"`
	Timezone string          `json:"timezone,omitempty"`
	Path     string          `json:"path"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// buildDescribeDoc snapshots the registry into the describe shape. It must
// not call any host import (plan §5.1: "fc_describe ... must not depend on
// any host call"), and it doesn't: it reads only in-memory registration
// state.
func buildDescribeDoc() describeDoc {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	doc := describeDoc{
		ABI:       1,
		Endpoints: make([]endpointJSON, 0, len(reg.endpoints)),
	}
	for _, e := range reg.endpoints {
		ej := endpointJSON{Method: e.Method, Path: e.Path, Auth: string(e.Auth)}
		ej.MaxBodyBytes = e.Options.maxBodyBytes
		ej.TimeoutMs = e.Options.timeoutMs
		if e.Options.cors != nil {
			ej.CORS = &corsJSON{
				Origins:          e.Options.cors.Origins,
				Methods:          e.Options.cors.Methods,
				Headers:          e.Options.cors.Headers,
				AllowCredentials: e.Options.cors.AllowCredentials,
			}
		}
		doc.Endpoints = append(doc.Endpoints, ej)
	}
	for _, s := range reg.subs {
		doc.Subscriptions = append(doc.Subscriptions, subJSON(s))
	}
	for _, s := range reg.scheds {
		doc.Schedules = append(doc.Schedules, schedJSON(s))
	}
	doc.Config = append(doc.Config, reg.config...)
	doc.Secrets = append(doc.Secrets, reg.secret...)
	doc.DB = append(doc.DB, reg.db...)
	doc.HTTPAllow = append(doc.HTTPAllow, reg.httpAllow...)
	doc.Emits = append(doc.Emits, reg.emits...)
	return doc
}

// describeJSON renders the current registry as the describe document bytes
// returned by fc_describe.
func describeJSON() []byte {
	doc := buildDescribeDoc()
	b, err := json.Marshal(doc)
	if err != nil {
		// Registration data is all plain strings/ints assembled by this
		// package; a marshal failure here would be an SDK bug, not a guest
		// error. Fail loudly rather than return a truncated describe doc.
		panic("fn: describe: " + err.Error())
	}
	return b
}
