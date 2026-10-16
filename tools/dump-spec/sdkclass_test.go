package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/sdk-classification.golden")

// wantApp is the reviewed list of application-facing operations. It is
// written out in full on purpose, independent of the operationId allow-list in
// sdkclass.go: changing the app surface is two deliberate edits, never one.
var wantApp = []string{
	"DELETE /api/applications/{appCode}/invitations/{id}",
	"DELETE /api/config/{app}/{section}/{property}",
	"GET /.well-known/jwks.json",
	"GET /.well-known/openid-configuration",
	"GET /api/applications/by-code/{code}",
	"GET /api/applications/{appCode}/invitations",
	"GET /api/applications/{appCode}/users",
	"GET /api/applications/{appCode}/users/{id}",
	"GET /api/applications/by-id/{id}/roles",
	"GET /api/applications/{id}",
	"GET /api/clients",
	"GET /api/clients/by-identifier/{identifier}",
	"GET /api/clients/{id}",
	"GET /api/config/{app}/{section}/{property}",
	"GET /api/connections",
	"GET /api/connections/{id}",
	"GET /api/dispatch-jobs",
	"GET /api/dispatch-jobs/event/{eventId}",
	"GET /api/dispatch-jobs/{id}",
	"GET /api/dispatch-jobs/{id}/attempts",
	"GET /api/dispatch-pools",
	"GET /api/dispatch-pools/{id}",
	"GET /api/email-domain-mappings/by-domain/{domain}",
	"GET /api/event-types",
	"GET /api/event-types/by-code/{code}",
	"GET /api/event-types/{id}",
	"GET /api/events",
	"GET /api/events/{id}",
	"GET /api/me",
	"GET /api/me/applications",
	"GET /api/me/clients",
	"GET /api/me/clients/{clientId}",
	"GET /api/me/clients/{clientId}/applications",
	"GET /api/platform-config/{app}",
	"GET /api/principals",
	"GET /api/principals/{id}",
	"GET /api/processes",
	"GET /api/processes/by-code/{code}",
	"GET /api/processes/{id}",
	"GET /api/roles",
	"GET /api/roles/by-application/{applicationId}",
	"GET /api/roles/by-code/{code}",
	"GET /api/roles/permissions",
	"GET /api/roles/permissions/{permission}",
	"GET /api/roles/{id}",
	"GET /api/scheduled-jobs",
	"GET /api/scheduled-jobs/by-code/{code}",
	"GET /api/scheduled-jobs/instances/{instanceId}",
	"GET /api/scheduled-jobs/instances/{instanceId}/logs",
	"GET /api/scheduled-jobs/{id}",
	"GET /api/subscriptions",
	"GET /api/subscriptions/{id}",
	"GET /oauth/userinfo",
	"POST /api/applications/{appCode}/connections/sync",
	"POST /api/applications/{appCode}/dispatch-pools/sync",
	"POST /api/applications/{appCode}/docs/sync",
	"POST /api/applications/{appCode}/event-types/sync",
	"POST /api/applications/{appCode}/invitations/{id}/resend",
	"POST /api/applications/{appCode}/openapi/sync",
	"POST /api/applications/{appCode}/principals/sync",
	"POST /api/applications/{appCode}/processes/sync",
	"POST /api/applications/{appCode}/roles/sync",
	"POST /api/applications/{appCode}/scheduled-jobs/sync",
	"POST /api/applications/{appCode}/subscriptions/sync",
	"POST /api/applications/{appCode}/users",
	"POST /api/audit-logs/batch",
	"POST /api/dispatch-jobs",
	"POST /api/dispatch-jobs/batch",
	"POST /api/event-types/{id}/versions",
	"POST /api/events",
	"POST /api/events/batch",
	"POST /api/principals/sync",
	"POST /api/principals/users",
	"POST /api/principals/{id}/reset-password",
	"POST /api/processes/sync",
	"POST /api/scheduled-jobs/instances/{instanceId}/complete",
	"POST /api/scheduled-jobs/instances/{instanceId}/log",
	"POST /oauth/introspect",
	"POST /oauth/revoke",
	"POST /oauth/token",
	"PUT /api/config/{app}/{section}/{property}",
	"PUT /api/principals/{id}",
	"PUT /api/principals/{id}/client-association",
}

func classOfExt(op *huma.Operation) string {
	if c, ok := op.Extensions[sdkExtension].(string); ok {
		return c
	}
	return classInternal
}

func opKey(spec *huma.OpenAPI) map[*huma.Operation]string {
	m := map[*huma.Operation]string{}
	for path, pi := range spec.Paths {
		for method, op := range map[string]*huma.Operation{"GET": pi.Get, "PUT": pi.Put, "POST": pi.Post, "DELETE": pi.Delete, "PATCH": pi.Patch, "OPTIONS": pi.Options, "HEAD": pi.Head, "TRACE": pi.Trace} {
			if op != nil {
				m[op] = method + " " + path
			}
		}
	}
	return m
}

func mustSpec(t *testing.T) *huma.OpenAPI {
	t.Helper()
	spec, err := buildSpec(true)
	require.NoError(t, err)
	return spec
}

// Every operation has exactly one class, recorded as x-sdk app | admin, or
// absent for internal. The lists themselves may not contradict each other or
// name an operation that does not exist (classifySpec checks both).
func TestEveryOperationHasExactlyOneClass(t *testing.T) {
	spec := mustSpec(t)
	keys := opKey(spec)
	require.NotEmpty(t, keys)
	seenID := map[string]bool{}
	for op, key := range keys {
		require.NotEmpty(t, op.OperationID, key)
		require.False(t, seenID[op.OperationID], "duplicate operationId %s", op.OperationID)
		seenID[op.OperationID] = true

		want, err := classOf(op)
		require.NoError(t, err, key)
		got := classOfExt(op)
		require.Equal(t, want, got, "%s: x-sdk disagrees with the lists", key)
		switch got {
		case classApp, classAdmin:
			require.Equal(t, got, op.Extensions[sdkExtension], key)
		case classInternal:
			_, has := op.Extensions[sdkExtension]
			require.False(t, has, "%s: internal operations carry no x-sdk", key)
		default:
			t.Fatalf("%s: unknown class %q", key, got)
		}
		inLists := 0
		for _, l := range []map[string]bool{appOps, internalOps} {
			if l[op.OperationID] {
				inLists++
			}
		}
		require.LessOrEqual(t, inLists, 1, "%s is in both the app and internal lists", op.OperationID)
	}
	// The three buckets partition the spec.
	n := map[string]int{}
	for op := range keys {
		n[classOfExt(op)]++
	}
	require.Equal(t, len(keys), n[classApp]+n[classAdmin]+n[classInternal])
	t.Logf("app=%d admin=%d internal=%d total=%d", n[classApp], n[classAdmin], n[classInternal], len(keys))
}

func TestAppOperationsMatchGoldenList(t *testing.T) {
	spec := mustSpec(t)
	var got []string
	for op, key := range opKey(spec) {
		if classOfExt(op) == classApp {
			got = append(got, key)
		}
	}
	sort.Strings(got)
	want := append([]string(nil), wantApp...)
	sort.Strings(want)
	require.Equal(t, want, got, "the app operation set changed: update wantApp (and docs/sdk-audit-and-python-sdk.md) deliberately")
}

// The full classification is snapshotted so a reviewer sees any operation
// that changes class (or appears) in the diff. `go test ./tools/dump-spec -update`.
func TestClassificationGolden(t *testing.T) {
	spec := mustSpec(t)
	var lines []string
	for op, key := range opKey(spec) {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", classOfExt(op), key, op.OperationID))
	}
	sort.Slice(lines, func(i, j int) bool {
		a, b := strings.SplitN(lines[i], "\t", 3), strings.SplitN(lines[j], "\t", 3)
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		return a[1] < b[1]
	})
	got := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("testdata", "sdk-classification.golden")
	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden; run `go test ./tools/dump-spec -update`")
	require.Equal(t, string(want), got, "SDK classification changed; review it, then `go test ./tools/dump-spec -update`")
}

// The generator profile is what the existing SDK code generators consume: it
// must stay free of SDK stubs and x-sdk, and every operation in it must exist
// in the full spec with the same operationId.
func TestGeneratorProfileHasNoSDKAdditions(t *testing.T) {
	gen, err := buildSpec(false)
	require.NoError(t, err)
	full := mustSpec(t)
	fullKeys := map[string]string{}
	for op, key := range opKey(full) {
		fullKeys[key] = op.OperationID
	}
	for op, key := range opKey(gen) {
		_, has := op.Extensions[sdkExtension]
		require.False(t, has, key)
		require.Equal(t, fullKeys[key], op.OperationID, key)
	}
	require.Greater(t, len(fullKeys), len(opKey(gen)), "the full spec adds the SDK stubs")
}

// RegisterSpec describes routes the server mounts as plain chi handlers; it
// must never also be registered against the live huma API.
func TestRegisterSpecNotWired(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(findRepoRoot(t), "internal", "server", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		require.NotContains(t, string(src), "RegisterSpec(", "%s wires a spec-only stub into the server", f)
	}
}

func splitFor(t *testing.T, class string) map[string]any {
	t.Helper()
	out, err := splitSpec(mustSpec(t), class)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(out, &doc))
	return doc
}

// Every $ref in a split document resolves, every operation in it has the
// requested class, and nothing unreferenced is carried along.
func TestSplitDocumentsAreSelfContained(t *testing.T) {
	full := mustSpec(t)
	classCount := map[string]int{}
	for op := range opKey(full) {
		classCount[classOfExt(op)]++
	}
	for _, class := range []string{classApp, classAdmin} {
		doc := splitFor(t, class)
		schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

		nOps := 0
		for _, v := range doc["paths"].(map[string]any) {
			for k, op := range v.(map[string]any) {
				if k == "parameters" {
					continue
				}
				require.Equal(t, class, op.(map[string]any)[sdkExtension], "%s document holds a foreign operation", class)
				nOps++
			}
		}
		require.Equal(t, classCount[class], nOps, "%s document misses operations", class)

		refs := map[string]bool{}
		collectRefs(doc["paths"], func(n string) { refs[n] = true })
		for name := range schemas {
			collectRefs(schemas[name], func(n string) { refs[n] = true })
		}
		for name := range refs {
			require.Contains(t, schemas, name, "%s document has a dangling $ref to %s", class, name)
		}
		// Reachability: every carried schema is referenced from a path (directly or transitively).
		direct := map[string]bool{}
		collectRefs(doc["paths"], func(n string) { direct[n] = true })
		reach := map[string]bool{}
		var walk func(string)
		walk = func(n string) {
			if reach[n] {
				return
			}
			reach[n] = true
			collectRefs(schemas[n], walk)
		}
		for n := range direct {
			walk(n)
		}
		require.Len(t, reach, len(schemas), "%s document carries unreferenced schemas", class)
	}
}

func TestSplitRejectsUnknownClass(t *testing.T) {
	_, err := splitSpec(mustSpec(t), "internal")
	require.Error(t, err)
}

// The app document must hold the full ingest and sync surface and none of the
// console surface (smoke check that the split follows the tags, beyond counts).
func TestAppDocumentContent(t *testing.T) {
	doc := splitFor(t, classApp)
	paths := doc["paths"].(map[string]any)
	for _, p := range []string{"/api/events/batch", "/api/dispatch-jobs/batch", "/api/audit-logs/batch", "/oauth/token", "/api/applications/{appCode}/roles/sync"} {
		require.Contains(t, paths, p)
	}
	for _, p := range []string{"/api/oauth-clients", "/api/service-accounts", "/api/functions", "/auth/webauthn/credentials"} {
		require.NotContains(t, paths, p)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	require.NotContains(t, schemas, "CreateOAuthClientRequest")
	require.Less(t, len(schemas), 200)
}

// The three app-facing routes that SDKs call but only plain chi handlers
// served: they are in the dumped spec, tagged app, with the shapes the
// handlers use. (sdk/spec_pg_test.go runs the real handlers against them.)
func TestIngestRoutesInSpec(t *testing.T) {
	spec := mustSpec(t)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	require.NoError(t, enc.Encode(spec))
	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	paths := doc["paths"].(map[string]any)

	type want struct {
		path, op, status, reqSchema, respSchema string
		required                                []string
	}
	for _, w := range []want{
		{"/api/dispatch-jobs", "createDispatchJob", "201", "CreateDispatchJobRequest", "DispatchJobCreatedResponse", []string{"code", "targetUrl", "payload", "serviceAccountId"}},
		{"/api/dispatch-jobs/batch", "batchCreateDispatchJobs", "201", "DispatchJobBatchRequest", "DispatchJobBatchResponse", nil},
		{"/api/audit-logs/batch", "batchIngestAuditLogs", "200", "AuditBatchRequest", "AuditBatchResponse", nil},
		{"/api/events", "createEvent", "201", "", "", nil},
	} {
		post := paths[w.path].(map[string]any)["post"].(map[string]any)
		require.Equal(t, w.op, post["operationId"])
		require.Equal(t, classApp, post[sdkExtension], w.path)
		require.Contains(t, post["responses"], w.status, w.path)
		if w.reqSchema != "" {
			body, _ := json.Marshal(post["requestBody"])
			require.Contains(t, string(body), "#/components/schemas/"+w.reqSchema)
			resp, _ := json.Marshal(post["responses"])
			require.Contains(t, string(resp), "#/components/schemas/"+w.respSchema)
		}
		if w.required != nil {
			s := doc["components"].(map[string]any)["schemas"].(map[string]any)[w.reqSchema].(map[string]any)
			var got []string
			for _, r := range s["required"].([]any) {
				got = append(got, r.(string))
			}
			require.ElementsMatch(t, w.required, got)
		}
	}
}
