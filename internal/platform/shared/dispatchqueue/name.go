package dispatchqueue

import (
	"errors"
	"fmt"
	"strings"
)

// TenantPlatform is the tenant segment reserved for platform-wide
// (client-less) dispatch jobs: they publish to {prefix}-platform-DEFAULT
// rather than to any client's lane.
//
// A client whose identifier were literally "platform" would collide with that
// lane, so the identifier is refused at client creation. Identifiers are
// immutable after create, so that one check closes the collision permanently.
// The pool-code resolver and the router-config document builder use this same
// literal for the platform tenant rather than each declaring a copy.
const TenantPlatform = "platform"

// SQSMaxNameLength is SQS's hard cap on a queue name, ".fifo" included.
const SQSMaxNameLength = 80

// DefaultPoolCode is the router's global fallback pool, and
// DefaultPoolSuffix the one structural read permitted on a composed pool code.
// A composed code's two halves may themselves contain hyphens, so nothing may
// split one back apart; a suffix test is unambiguous because the literal is
// fixed.
const (
	DefaultPoolCode   = "DEFAULT-POOL"
	DefaultPoolSuffix = "-" + DefaultPoolCode
)

// ComposePoolCode is the wire poolCode for a dispatch pool:
// {clientIdentifier}-{poolCode} when the pool is client-owned, and
// platform-{poolCode} when it is platform-level.
//
// Platform-level pools take the platform- prefix rather than going out bare.
// The router merges this platform's document with other config sources and
// pools merge by code, first definition winning — so an unprefixed platform
// pool named, say, WEBHOOKS would silently inherit another tenant's settings
// of the same name, with only a merge-conflict log line to show for it.
//
// This is the one place the composition happens: the scheduler stamps a
// claimed job's pool code with it and the router-config document names its
// pools with it, so a stamped code always matches a pool in the document.
func ComposePoolCode(poolCode string, clientIdentifier *string) string {
	tenant := TenantPlatform
	if clientIdentifier != nil && *clientIdentifier != "" {
		tenant = *clientIdentifier
	}
	return tenant + "-" + poolCode
}

// IsDefaultPoolCode reports whether code names a fallback pool — the global
// DefaultPoolCode or any per-tenant {identifier}-DEFAULT-POOL, of which
// platform-DEFAULT-POOL is simply one more instance.
func IsDefaultPoolCode(code string) bool {
	return code == DefaultPoolCode || strings.HasSuffix(code, DefaultPoolSuffix)
}

// ErrTenantRequired is returned by ComposeName for a blank tenant. A blank one
// would compose a shared lane rather than the per-client isolation the name
// exists to provide, so it is refused rather than papered over.
var ErrTenantRequired = errors.New("dispatchqueue: tenant is required to compose a queue name")

// NameTooLongError is returned by ComposeName when the SQS form would exceed
// SQSMaxNameLength. tnt_clients.identifier is varchar(100), comfortably wider
// than SQS allows once the fixed segments are added, so this is a reachable
// condition rather than defensive dead code. The document builder omits that
// one tenant rather than failing the whole document.
type NameTooLongError struct {
	Tenant   string
	Composed string
}

func (e *NameTooLongError) Error() string {
	return fmt.Sprintf("dispatch queue name %q (%d chars) exceeds SQS's %d-character limit for tenant %q",
		e.Composed, len(e.Composed), SQSMaxNameLength, e.Tenant)
}

// ComposeName builds a dispatch queue's name: {prefix}-{tenant}-{priority},
// ".fifo"-suffixed when sqs is true — FC-staging-acme-DEFAULT.fifo,
// FC-staging-acme-HIGH_PRIORITY.fifo.
//
// Every deployed dispatch queue is FIFO (per-group ordering is load-bearing),
// so sqs alone decides both the suffix and whether the length cap applies.
//
// A blank prefix omits that segment rather than failing: the requirement that
// a deployed SQS environment set one is enforced once, at startup, by
// SettingsFromEnv. A Postgres (dev) name with no prefix configured is a
// normal, if less tidy, name.
//
// Only the tenant segment is sanitised, exactly as the existing convention
// sanitises: "_", "." and space each become "-". A conforming client
// identifier is already a lower-case slug containing none of them, so this is
// a no-op in the ordinary case and is kept defensively. The priority segment
// is never sanitised — HIGH_PRIORITY keeps its underscore, which is legal in
// an SQS queue name, and a Priority is always one of the two constants.
func ComposeName(prefix, tenant string, priority Priority, sqs bool) (string, error) {
	if strings.TrimSpace(tenant) == "" {
		return "", ErrTenantRequired
	}
	if priority == "" {
		priority = PriorityDefault
	}
	var b strings.Builder
	if strings.TrimSpace(prefix) != "" {
		b.WriteString(prefix)
		b.WriteString("-")
	}
	b.WriteString(sanitiseTenant(tenant))
	b.WriteString("-")
	b.WriteString(string(priority))
	if !sqs {
		return b.String(), nil
	}
	b.WriteString(".fifo")
	composed := b.String()
	if len(composed) > SQSMaxNameLength {
		return "", &NameTooLongError{Tenant: tenant, Composed: composed}
	}
	return composed, nil
}

// sanitiseTenant applies the tenant sanitisation the naming convention
// requires: "_", "." and space each become "-".
func sanitiseTenant(tenant string) string {
	return strings.NewReplacer("_", "-", ".", "-", " ", "-").Replace(tenant)
}
