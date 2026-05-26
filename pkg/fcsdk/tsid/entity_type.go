package tsid

// EntityType enumerates the well-known prefixes used across the
// FlowCatalyst platform. Mirrors the Rust crate fc_common::tsid and the
// TypeScript / Laravel SDKs — all four SDKs share the same wire format.
//
// New variants MUST be added to every SDK so the format stays consistent.
type EntityType int

const (
	Client EntityType = iota
	Principal
	Application
	ServiceAccount
	Role
	Permission
	OAuthClient
	AuthCode
	LoginAttempt
	ClientAuthConfig
	AppClientConfig
	IdpRoleMapping
	CorsOrigin
	AnchorDomain
	IdentityProvider
	EmailDomainMapping
	ClientAccessGrant
	EventType
	Event
	EventRead
	Connection
	Subscription
	DispatchPool
	DispatchJob
	DispatchJobRead
	Schema
	AuditLog
	PlatformConfig
	ConfigAccess
	PasswordResetToken
	WebauthnCredential
	ScheduledJob
	ScheduledJobInstance
	ScheduledJobInstanceLog
	ApplicationOpenApiSpec
	Process
)

// Prefix returns the 3-character platform prefix for the entity type.
// Mirrors crates/fc-common/src/tsid.rs::EntityType::prefix.
func (e EntityType) Prefix() string {
	switch e {
	case Client:
		return "clt"
	case Principal:
		return "prn"
	case Application:
		return "app"
	case ServiceAccount:
		return "sac"
	case Role:
		return "rol"
	case Permission:
		return "prm"
	case OAuthClient:
		return "oac"
	case AuthCode:
		return "acd"
	case LoginAttempt:
		return "lat"
	case ClientAuthConfig:
		return "cac"
	case AppClientConfig:
		return "apc"
	case IdpRoleMapping:
		return "irm"
	case CorsOrigin:
		return "cor"
	case AnchorDomain:
		return "anc"
	case IdentityProvider:
		return "idp"
	case EmailDomainMapping:
		return "edm"
	case ClientAccessGrant:
		return "gnt"
	case EventType:
		return "evt"
	case Event:
		return "evn"
	case EventRead:
		return "evr"
	case Connection:
		return "con"
	case Subscription:
		return "sub"
	case DispatchPool:
		return "dpl"
	case DispatchJob:
		return "djb"
	case DispatchJobRead:
		return "djr"
	case Schema:
		return "sch"
	case AuditLog:
		return "aud"
	case PlatformConfig:
		return "pcf"
	case ConfigAccess:
		return "cfa"
	case PasswordResetToken:
		return "prt"
	case WebauthnCredential:
		return "pkc"
	case ScheduledJob:
		return "sjb"
	case ScheduledJobInstance:
		return "sji"
	case ScheduledJobInstanceLog:
		return "sjl"
	case ApplicationOpenApiSpec:
		return "oas"
	case Process:
		return "prc"
	}
	return "unk"
}
