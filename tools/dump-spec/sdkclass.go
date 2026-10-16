package main

import (
	"fmt"
	"sort"

	"github.com/danielgtaylor/huma/v2"
)

// SDK classification of the platform API (docs/sdk-audit-and-python-sdk.md
// A.2, A.8 and "Spec tagging as built").
//
// Every operation in the dumped spec has exactly one class, carried as the
// `x-sdk` vendor extension:
//
//	app      what an application SDK calls (sync, ingest, token/OIDC, the
//	         reads and app-delegated writes the SDKs and apps use)
//	admin    admin-console / operator surface: belongs to an optional admin
//	         client, not an application SDK
//	(absent) internal: legacy aliases and operations that are neither (browser
//	         ceremonies, the function runner's control plane, router-to-platform
//	         config). Recorded in internalOps below, so it is a decision, not a
//	         default.
//
// app is an explicit allow-list by operationId. admin is by tag (a whole console
// area). internal is an explicit list by operationId and wins over the admin
// tag. A new operation whose tag is in none of the lists, or that matches the
// lists inconsistently, fails the dump: classification is always a decision.

const (
	classApp      = "app"
	classAdmin    = "admin"
	classInternal = "internal"

	sdkExtension = "x-sdk"
)

// appOps is the allow-list of application-facing operations, by operationId.
var appOps = idSet(
	// Declarative definition sync (the application path for definitions).
	"syncRoles", "syncEventTypes", "syncSubscriptions", "syncConnections", "syncDispatchPools",
	"syncProcesses", "syncProcessesByBody", "syncPrincipals", "syncScheduledJobs", "syncAppDocs", "syncOpenapi",
	"syncUsers", // POST /api/principals/sync: user sync

	// Ingest (outbox processors and direct publishers).
	"createEvent", "batchIngestEvents",
	"createDispatchJob", "batchCreateDispatchJobs",
	"batchIngestAuditLogs",

	// Scheduled-job runner: read, completion, log.
	"listScheduledJobs", "getScheduledJob", "getScheduledJobByCode",
	"getScheduledJobInstance", "listScheduledJobInstanceLogs",
	"completeScheduledJobInstance", "writeScheduledJobInstanceLog",

	// Reads of definitions (A.8 "Reads").
	"listRoles", "getRole", "getRoleByCode", "getRolesByApplication", "listPermissions", "getPermission",
	"listEventTypes", "getEventType", "getEventTypeByCode",
	"listSubscriptions", "getSubscription",
	"listConnections", "getConnection",
	"listProcesses", "getProcess", "getProcessByCode",
	"listDispatchPools", "getDispatchPool",
	"getApplication", "getApplicationByCode", "listApplicationRoles",
	"listClients", "getClient", "getClientByIdentifier",

	// Operations the Inhance apps use through the Laravel request() escape hatch.
	"addEventTypeVersion",
	"getPlatformConfigProperty", "setPlatformConfigProperty", "deletePlatformConfigProperty", "listPlatformConfigProperties",
	"getEmailDomainMappingByDomain",
	"setPrincipalClientAssociation",

	// App-delegated user management (A.3: used by all three Inhance apps).
	"getPrincipal", "listPrincipals", "createUser", "updatePrincipal", "resetPrincipalPassword",

	// Who am I (served by plain chi handlers; described by me.RegisterSpec).
	"getMe", "listMyApplications", "listMyClients", "getMyClient", "listMyClientApplications",

	// Token, JWKS and OIDC (plain chi handlers; described by oauthapi.RegisterSpec).
	"oauthToken", "oauthIntrospect", "oauthRevoke", "oauthUserinfo", "openidConfiguration", "jwks",
)

// internalOps are operations that are deliberately neither app nor admin.
var internalOps = idSet(
	// Legacy aliases and duplicates of a canonical operation (A.2 class 3).
	"listDispatchJobsRaw", "listDispatchJobsRawAlias", "getDispatchJobRaw", // /raw, /list-raw
	"dispatchJobsByEventAlias",            // by-event vs event
	"listEventsRaw", "listEventsRawAlias", // /raw, /list-raw
	"regenerateOAuthClientSecret",                             // alias of rotate-secret
	"regenerateServiceAccountSigningSecret_regenerate-secret", // alias of regenerate-signing-secret
	"regenerateServiceAccountAuthToken_regenerate-token",      // alias of regenerate-auth-token
	"searchClientsByQuery",                                    // GET twin of POST /clients/search
	"listAuditLogsRecent",                                     // alias for list
	"addEventTypeSchema",                                      // alias of addEventTypeVersion

	// Function runner control plane: machine-to-machine from the function
	// runner to the platform, not console and not application surface.
	"getFunctionRunnerArtifact", "getFunctionRunnerDesired", "postFunctionRunnerDispatchJob",
	"postFunctionRunnerEvent", "postFunctionRunnerHeartbeat",

	// Passkey browser ceremonies (session-bound; the credential list/delete
	// operations stay admin).
	"webauthnAuthenticateBegin", "webauthnAuthenticateComplete", "webauthnRegisterBegin", "webauthnRegisterComplete",

	// Served to the router host / browsers, not to a console or an application.
	"getRouterConfig", "publicAllowedOrigins",
)

// adminTags are the console areas whose operations are admin unless the
// operationId is listed above.
var adminTags = idSet(
	"anchor-domains", "applications", "audit-logs", "auth-configs", "clients", "connections",
	"cors-origins", "dispatch-jobs", "dispatch-pools", "docs", "email-domain-mappings",
	"event-types", "events", "function-domains", "functions", "identity-providers",
	"idp-role-mappings", "login-attempts", "oauth-clients", "platform-config",
	"portal-apps", "portal-users", "principals", "processes", "reset-approvals",
	"roles", "scheduled-jobs", "service-accounts", "subscriptions", "webauthn",
)

func idSet(ids ...string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// classOf returns the class of one operation, or an error when the lists do
// not decide it.
func classOf(op *huma.Operation) (string, error) {
	inApp, inInternal := appOps[op.OperationID], internalOps[op.OperationID]
	switch {
	case inApp && inInternal:
		return "", fmt.Errorf("%s is in both the app and internal lists", op.OperationID)
	case inApp:
		return classApp, nil
	case inInternal:
		return classInternal, nil
	}
	if len(op.Tags) > 0 && adminTags[op.Tags[0]] {
		return classAdmin, nil
	}
	return "", fmt.Errorf("%s (tag %v) is unclassified: add it to appOps, internalOps, or its tag to adminTags in tools/dump-spec/sdkclass.go", op.OperationID, op.Tags)
}

// operations lists every operation in spec in a stable order.
func operations(spec *huma.OpenAPI) []*huma.Operation {
	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var ops []*huma.Operation
	for _, p := range paths {
		pi := spec.Paths[p]
		for _, op := range []*huma.Operation{pi.Get, pi.Put, pi.Post, pi.Delete, pi.Options, pi.Head, pi.Patch, pi.Trace} {
			if op != nil {
				ops = append(ops, op)
			}
		}
	}
	return ops
}

// classifySpec stamps x-sdk on every app and admin operation and verifies the
// lists: every listed operationId must exist, every operation must be decided.
func classifySpec(spec *huma.OpenAPI) error {
	seen := map[string]bool{}
	for _, op := range operations(spec) {
		seen[op.OperationID] = true
		class, err := classOf(op)
		if err != nil {
			return err
		}
		if class == classInternal {
			continue
		}
		if op.Extensions == nil {
			op.Extensions = map[string]any{}
		}
		op.Extensions[sdkExtension] = class
	}
	for _, list := range []map[string]bool{appOps, internalOps} {
		for id := range list {
			if !seen[id] {
				return fmt.Errorf("classification lists name operationId %q, which is not in the spec", id)
			}
		}
	}
	return nil
}
