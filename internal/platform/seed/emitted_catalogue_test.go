package seed

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// eventCodePattern is a platform event-type code:
// platform:{subdomain}:{aggregate}:{event}.
var eventCodePattern = regexp.MustCompile(`^platform:[a-z-]+:[a-z-]+:[a-z-]+$`)

// knownUncataloguedEventTypes are event types the platform emits that the
// seeded catalogue does not list yet (found 2026-10-10 while aligning the
// client events, docs/platform-bug-fixes.md "New findings"). A subscription
// cannot pick them from the catalogue. The list may only shrink: an entry that
// becomes catalogued, or stops being emitted, fails the test until removed.
var knownUncataloguedEventTypes = map[string]bool{
	"platform:admin:anchor-domain:created":                 true,
	"platform:admin:anchor-domain:deleted":                 true,
	"platform:admin:anchor-domain:updated":                 true,
	"platform:admin:auth-config:created":                   true,
	"platform:admin:auth-config:deleted":                   true,
	"platform:admin:auth-config:updated":                   true,
	"platform:admin:dispatch-pool:activated":               true,
	"platform:admin:dispatch-pool:suspended":               true,
	"platform:admin:email-domain-mapping:created":          true,
	"platform:admin:email-domain-mapping:deleted":          true,
	"platform:admin:email-domain-mapping:provider-changed": true,
	"platform:admin:email-domain-mapping:updated":          true,
	"platform:admin:function-alias:deleted":                true,
	"platform:admin:function-alias:set":                    true,
	"platform:admin:function-domain:claimed":               true,
	"platform:admin:function-domain:released":              true,
	"platform:admin:function-route:deleted":                true,
	"platform:admin:function-route:set":                    true,
	"platform:admin:function-schedule:wired":               true,
	"platform:admin:function-setting:deleted":              true,
	"platform:admin:function-setting:set":                  true,
	"platform:admin:function-version:published":            true,
	"platform:admin:function-version:retired":              true,
	"platform:admin:function:created":                      true,
	"platform:admin:function:deleted":                      true,
	"platform:admin:function:promoted":                     true,
	"platform:admin:function:updated":                      true,
	"platform:admin:identity-provider:created":             true,
	"platform:admin:identity-provider:deleted":             true,
	"platform:admin:identity-provider:updated":             true,
	"platform:admin:idp-role-mapping:created":              true,
	"platform:admin:idp-role-mapping:deleted":              true,
	"platform:admin:oauth-client:activated":                true,
	"platform:admin:oauth-client:created":                  true,
	"platform:admin:oauth-client:deactivated":              true,
	"platform:admin:oauth-client:deleted":                  true,
	"platform:admin:oauth-client:previous-secret-revoked":  true,
	"platform:admin:oauth-client:secret-rotated":           true,
	"platform:admin:oauth-client:updated":                  true,
	"platform:admin:passkey:authenticated":                 true,
	"platform:admin:passkey:registered":                    true,
	"platform:admin:passkey:revoked":                       true,
	"platform:admin:platform-config:access-granted":        true,
	"platform:admin:platform-config:access-revoked":        true,
	"platform:admin:platform-config:property-set":          true,
	"platform:admin:process:archived":                      true,
	"platform:admin:process:created":                       true,
	"platform:admin:process:deleted":                       true,
	"platform:admin:process:updated":                       true,
	"platform:admin:processes:synced":                      true,
	"platform:admin:role:created":                          true,
	"platform:admin:role:deleted":                          true,
	"platform:admin:role:permission-granted":               true,
	"platform:admin:role:permission-revoked":               true,
	"platform:admin:role:updated":                          true,
	"platform:admin:roles:synced":                          true,
	"platform:admin:scheduled-job:archived":                true,
	"platform:admin:scheduled-job:created":                 true,
	"platform:admin:scheduled-job:deleted":                 true,
	"platform:admin:scheduled-job:fired-manually":          true,
	"platform:admin:scheduled-job:paused":                  true,
	"platform:admin:scheduled-job:resumed":                 true,
	"platform:admin:scheduled-job:updated":                 true,
	"platform:admin:scheduledjobs:synced":                  true,
	"platform:developer:application-openapi:synced":        true,
	"platform:iam:serviceaccount:deactivated":              true,
	"platform:iam:user:developer-credential-revoked":       true,
	"platform:iam:user:developer-credential-set":           true,
	"platform:messaging:dispatch-job:cancelled":            true,
	"platform:messaging:dispatch-job:completed":            true,
	"platform:messaging:dispatch-jobs:resent":              true,
	"platform:portal:app:created":                          true,
	"platform:portal:app:deleted":                          true,
	"platform:portal:app:updated":                          true,
	"platform:portal:identity:app-granted":                 true,
	"platform:portal:identity:app-revoked":                 true,
	"platform:portal:identity:deleted":                     true,
	"platform:portal:identity:ensured":                     true,
	"platform:portal:identity:status-set":                  true,
}

// emittedEventTypes collects the event-type codes the platform's use cases
// emit: every string constant declared in an events.go under
// internal/platform whose value is an event-type code (permission constants,
// which share the shape, are named perm*/…Perm and are skipped).
func emittedEventTypes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "events.go" {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) || strings.HasPrefix(name.Name, "perm") || strings.HasSuffix(name.Name, "Perm") {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					code, err := strconv.Unquote(lit.Value)
					if err == nil && eventCodePattern.MatchString(code) {
						out[code] = path + ": " + name.Name
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan events.go files: %v", err)
	}
	if len(out) < 100 {
		t.Fatalf("found only %d emitted event types; the scan is broken", len(out))
	}
	return out
}

// Every event type a use case emits is in the seeded catalogue (so a
// subscription can pick it), except the known gaps above. The client events
// were catalogued as platform:iam:client:* while the client use cases emit
// platform:admin:client:*.
func TestEveryEmittedEventTypeIsCatalogued(t *testing.T) {
	catalogued := map[string]bool{}
	for _, d := range PlatformEventTypes() {
		catalogued[d.Code] = true
	}
	emitted := emittedEventTypes(t)
	var missing []string
	for code, where := range emitted {
		if !catalogued[code] && !knownUncataloguedEventTypes[code] {
			missing = append(missing, code+" ("+where+")")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("emitted but not in the seeded catalogue: %s", m)
	}
	for code := range knownUncataloguedEventTypes {
		if catalogued[code] {
			t.Errorf("%s is catalogued now; remove it from knownUncataloguedEventTypes", code)
		}
		if _, ok := emitted[code]; !ok {
			t.Errorf("%s is no longer emitted; remove it from knownUncataloguedEventTypes", code)
		}
	}
}
