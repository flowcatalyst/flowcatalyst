package fn

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
)

type kvGetMeta struct {
	Key string `json:"key"`
}

type kvGetResult struct {
	Found bool `json:"found"`
}

// ConfigValue is a declared configuration key (plan §5.3 op 2, config.get).
type ConfigValue struct{ key string }

// Key returns the declared configuration key name.
func (c *ConfigValue) Key() string { return c.key }

// Get fetches the current value from the platform's desired state. found is
// false when the key has no value bound (distinct from an empty string
// value, and distinct from an error).
func (c *ConfigValue) Get(ctx context.Context) (value string, found bool, err error) {
	return getKV(opConfigGet, c.key)
}

// Config declares that this function reads configuration key key. Call it
// from init(); the returned handle's Get reads the current value at
// runtime.
func Config(key string) *ConfigValue {
	reg.mu.Lock()
	reg.config = append(reg.config, key)
	reg.mu.Unlock()
	return &ConfigValue{key: key}
}

// SecretValue is a declared secret key (plan §5.3 op 3, secret.get).
type SecretValue struct{ key string }

// Key returns the declared secret key name.
func (s *SecretValue) Key() string { return s.key }

// Get fetches the current value. Secret values are never logged by the SDK.
func (s *SecretValue) Get(ctx context.Context) (value string, found bool, err error) {
	return getKV(opSecretGet, s.key)
}

// Secret declares that this function reads secret key key.
func Secret(key string) *SecretValue {
	reg.mu.Lock()
	reg.secret = append(reg.secret, key)
	reg.mu.Unlock()
	return &SecretValue{key: key}
}

func getKV(op opCode, key string) (value string, found bool, err error) {
	mb, merr := json.Marshal(kvGetMeta{Key: key})
	if merr != nil {
		return "", false, merr
	}
	rm, rb, hostErr, err := currentHost.call(op, mb, nil)
	if err != nil {
		return "", false, err
	}
	if hostErr != nil {
		return "", false, hostErr
	}
	var out kvGetResult
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return "", false, uerr
		}
	}
	if !out.Found {
		return "", false, nil
	}
	return string(rb), true, nil
}

// DBBinding is a declared database binding (plan §5.3 ops 6-10).
type DBBinding struct {
	name string
	once sync.Once
	db   *sql.DB
}

// Name returns the declared database binding name.
func (d *DBBinding) Name() string { return d.name }

// DB declares that this function uses database binding name (the platform
// resolves it to a DSN in settings; plan §4).
func DB(name string) *DBBinding {
	reg.mu.Lock()
	reg.db = append(reg.db, name)
	reg.mu.Unlock()
	return &DBBinding{name: name}
}

// HTTPAllow declares outbound HTTP hosts this function may reach (plan
// §5.4). Wildcards like "*.acme.com" are supported.
func HTTPAllow(hosts ...string) {
	reg.mu.Lock()
	reg.httpAllow = append(reg.httpAllow, hosts...)
	reg.mu.Unlock()
}

// Emits declares event types this function may publish via Emit.
func Emits(eventTypes ...string) {
	reg.mu.Lock()
	reg.emits = append(reg.emits, eventTypes...)
	reg.mu.Unlock()
}
