package secretref

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// writeCode is the code of a CheckWrite refusal (a usecase validation).
func writeCode(err error) string {
	if ue := usecase.AsError(err); ue != nil {
		return ue.Code
	}
	return ""
}

// TA1: the matcher. A scoped entry admits a reference only at the owner's own
// application (and client); a plain entry is the raw prefix it always was.
func TestMatchEntry(t *testing.T) {
	billing := Owner{Application: "billing"}
	billingAcme := Owner{Application: "billing", Client: "clt_acme"}
	cases := []struct {
		name  string
		entry string
		ref   string
		owner Owner
		want  bool
	}{
		{"app matches its own segment", "aws-sm://prod/{app}/", "aws-sm://prod/billing/db#password", billing, true},
		{"another application's segment", "aws-sm://prod/{app}/", "aws-sm://prod/orders/db", billing, false},
		{"a sibling name is not the segment", "aws-sm://prod/{app}/", "aws-sm://prod/billing-admin/db", billing, false},
		{"no application supplied", "aws-sm://prod/{app}/", "aws-sm://prod/billing/db", Owner{}, false},
		{"client only supplied is not enough for app", "aws-sm://prod/{app}/", "aws-sm://prod/billing/db", Owner{Client: "clt_acme"}, false},
		{"app and client both", "aws-sm://prod/{app}/{client}/", "aws-sm://prod/billing/clt_acme/db", billingAcme, true},
		{"client missing", "aws-sm://prod/{app}/{client}/", "aws-sm://prod/billing/clt_acme/db", billing, false},
		{"another client", "aws-sm://prod/{app}/{client}/", "aws-sm://prod/billing/clt_other/db", billingAcme, false},
		{"client placeholder alone", "aws-sm://prod/{client}/", "aws-sm://prod/clt_acme/x", billingAcme, true},
		{"plain entry, any caller", "aws-sm://shared/", "aws-sm://shared/x", Owner{}, true},
		{"plain entry, another caller", "aws-sm://shared/", "aws-sm://shared/x", billingAcme, true},
		{"plain entry miss", "aws-sm://shared/", "aws-sm://other/x", billingAcme, false},
		{"plain entry without slash admits siblings", "aws-sm://prod/shared", "aws-sm://prod/shared-admin/x", Owner{}, true},
		{"app value containing a slash never matches", "aws-sm://prod/{app}/", "aws-sm://prod/a/b/db", Owner{Application: "a/b"}, false},
		{"app value containing a slash, ref under its first segment", "aws-sm://prod/{app}/", "aws-sm://prod/a/db", Owner{Application: "a/b"}, false},
		{"dot-dot segment", "aws-sm://prod/{app}/", "aws-sm://prod/../db", Owner{Application: ".."}, false},
		{"ref ends at the segment", "aws-sm://prod/{app}", "aws-sm://prod/billing", billing, true},
		{"ref ends at the segment, with a key", "aws-sm://prod/{app}", "aws-sm://prod/billing#k", billing, true},
		{"ref ends at the segment, sibling", "aws-sm://prod/{app}", "aws-sm://prod/billing2", billing, false},
		{"placeholder not a whole segment never matches", "aws-sm://prod/x{app}/", "aws-sm://prod/xbilling/db", billing, false},
		{"placeholder not followed by a slash never matches", "aws-sm://prod/{app}-db/", "aws-sm://prod/billing-db/x", billing, false},
		{"parameter store", "aws-ps:///env/{app}/", "aws-ps:///env/billing/key", billing, true},
		{"parameter store other app", "aws-ps:///env/{app}/", "aws-ps:///env/orders/key", billing, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchEntry(tc.entry, tc.ref, tc.owner, false), "%s vs %s for %+v", tc.entry, tc.ref, tc.owner)
		})
	}
}

// The service applies the same rule at the check and at the resolve, and the
// message does not list the placeholders' expansions of other applications.
func TestAllowedAndResolveShareTheOwnerRule(t *testing.T) {
	f := newFake()
	f.secrets["prod/billing/db"] = `{"password":"p"}`
	f.secrets["prod/orders/db"] = `{"password":"o"}`
	s := New(Config{Allow: []string{"aws-sm://prod/{app}/"}}, f)
	billing, orders := Owner{Application: "billing"}, Owner{Application: "orders"}

	require.NoError(t, s.Allowed("aws-sm://prod/billing/db", billing))
	require.NoError(t, s.CheckWrite("aws-sm://prod/billing/db#password", billing))
	v, err := s.Resolve(context.Background(), "aws-sm://prod/billing/db#password", billing)
	require.NoError(t, err)
	assert.Equal(t, "p", v)

	assert.Equal(t, CodeNotAllowed, CodeOf(s.Allowed("aws-sm://prod/orders/db", billing)))
	assert.Equal(t, CodeNotAllowed, writeCode(s.CheckWrite("aws-sm://prod/orders/db#password", billing)))
	_, err = s.Resolve(context.Background(), "aws-sm://prod/orders/db#password", billing)
	assert.Equal(t, CodeNotAllowed, CodeOf(err), "a reference to another application's secret does not resolve")
	// The value is cached for orders, but a reference stored under billing
	// does not read it: the check runs before the cache.
	_, err = s.Resolve(context.Background(), "aws-sm://prod/orders/db#password", orders)
	require.NoError(t, err)
	_, err = s.Resolve(context.Background(), "aws-sm://prod/orders/db#password", billing)
	assert.Equal(t, CodeNotAllowed, CodeOf(err))
	_, err = s.Resolve(context.Background(), "aws-sm://prod/billing/db#password", Owner{})
	assert.Equal(t, CodeNotAllowed, CodeOf(err), "no owner supplied")
	assert.Contains(t, err.Error(), "outside what this application/client may use")
}

// The pre-check (an operation's Validate, before the row is known) refuses only
// what no owner could be admitted for.
func TestPrecheckWrite(t *testing.T) {
	s := New(Config{Allow: []string{"aws-sm://prod/{app}/", "aws-sm://shared/"}}, nil)
	assert.NoError(t, s.PrecheckWrite("aws-sm://prod/anyone/db"))
	assert.NoError(t, s.PrecheckWrite("aws-sm://shared/x"))
	assert.Equal(t, CodeNotAllowed, writeCode(s.PrecheckWrite("aws-sm://other/x")))
	assert.Equal(t, CodeNotAllowed, writeCode(s.PrecheckWrite("aws-sm://prod//db")), "an empty segment is no application")
	assert.Equal(t, CodeUnsupported, writeCode(s.PrecheckWrite("vault://x")))
	assert.NoError(t, s.PrecheckWrite("a plain secret"))
}

// A plain entry that does not end in "/" is logged once, by name; nothing about
// how it matches changes.
func TestWarnLooseEntries(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	WarnLooseEntries([]string{"aws-sm://prod/shared", "aws-sm://ok/", "aws-sm://prod/{app}/", "aws-sm://prod/x{app}/", "aws-ps:///env/{app}"})
	out := buf.String()
	assert.Equal(t, 2, strings.Count(out, "level=WARN"), out)
	assert.Contains(t, out, "entry=aws-sm://prod/shared")
	assert.Contains(t, out, "entry=aws-sm://prod/x{app}/")
	assert.NotContains(t, out, "entry=aws-sm://ok/")

	s := New(Config{Allow: []string{"aws-sm://prod/shared"}}, nil)
	assert.NoError(t, s.Allowed("aws-sm://prod/shared-admin/x", Owner{}), "still a raw prefix")

	t.Setenv("FC_SECRET_REFERENCE_ALLOW", "aws-sm://loose, aws-sm://fine/")
	buf.Reset()
	_ = ConfigFromEnv()
	assert.Equal(t, 1, strings.Count(buf.String(), "level=WARN"), "the allow-list's load logs it")
	assert.Contains(t, buf.String(), "entry=aws-sm://loose")
}
