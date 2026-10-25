package ingestion_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The outbound guard on source addresses (owner ruling R4b): loopback,
// link-local, unspecified, multicast, the cloud metadata addresses and names,
// and numeric host forms are refused; private networks, where on-premises
// sources live, are not.
func TestCheckSourceHost(t *testing.T) {
	for _, host := range []string{
		"127.0.0.1", "127.1.2.3", "::1", "[::1]", "localhost", "LOCALHOST", "db.localhost", "localhost.",
		"169.254.169.254", "169.254.170.2", "fe80::1", "fe80::1%eth0", "0.0.0.0", "::", "224.0.0.1",
		"fd00:ec2::254", "100.100.100.200", "::ffff:127.0.0.1", "::ffff:169.254.169.254",
		"metadata.google.internal", "metadata", "Metadata.Google.Internal.", "instance-data", "metadata.goog",
		"2130706433", "0x7f000001", "0x7f.1", "127.1", "017700000001",
	} {
		assert.Error(t, ingestion.CheckSourceHost(host), host)
	}
	for _, host := range []string{
		"10.1.2.3", "192.168.1.20", "172.16.0.9", "fd12:3456::1", "100.64.0.1",
		"sftp.example.com", "db-1.internal", "orders.corp", "1st-db.example.com", "8.8.8.8",
	} {
		assert.NoError(t, ingestion.CheckSourceHost(host), host)
	}
	assert.Error(t, ingestion.CheckSourceHost(""), "a host is required")
}

// Every outbound address of every kind passes the guard at save, reported at
// its field.
func TestValidate_SourceAddresses(t *testing.T) {
	cases := []struct {
		name, source, split, field string
	}{
		{"sftp host", `{"kind":"sftp","host":"127.0.0.1","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"}`, `{"kind":"none"}`, "source.host"},
		{"sftp metadata name", `{"kind":"sftp","host":"metadata.google.internal","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"}`, `{"kind":"none"}`, "source.host"},
		{"rabbitmq", `{"kind":"rabbitmq","uri":"amqp://localhost:5672","queue":"q","username":"u","password":{"name":"pw"}}`, `{"kind":"none"}`, "source.uri"},
		{"api-poll url", apiPoll(`http://169.254.169.254/latest/meta-data`, `{"kind":"none"}`), `{"kind":"json","arrayPath":"$"}`, "source.url"},
		{"api-poll token url", apiPoll(`https://api.example.com/x`, `{"kind":"oauth-client-credentials","tokenUrl":"http://[::1]/token","clientId":"c","clientSecret":{"name":"s"},"credentialsIn":"BODY"}`), `{"kind":"json","arrayPath":"$"}`, "source.auth.tokenUrl"},
		{"database host", dbPoll("jdbc:postgresql://127.0.0.1:5432/orders"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"database second host", dbPoll("jdbc:postgresql://db1.example.com:5432,169.254.169.254:5432/orders"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"database without a host", dbPoll("jdbc:h2:mem:orders"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"database file", dbPoll("jdbc:sqlite:/etc/passwd"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"oracle tns alias", dbPoll("jdbc:oracle:thin:@ORDERS"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"oracle description", dbPoll("jdbc:oracle:thin:@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521)))"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"sql server property", dbPoll("jdbc:sqlserver://;serverName=127.0.0.1;databaseName=orders"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"mysql address form", dbPoll("jdbc:mysql://address=(protocol=tcp)(host=localhost)(port=3306)/orders"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"jdbc socket factory", dbPoll("jdbc:postgresql://db.example.com/orders?socketFactory=org.springframework.context.support.ClassPathXmlApplicationContext"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"jdbc local infile", dbPoll("jdbc:mysql://db.example.com/orders?allowLoadLocalInfile=true"), `{"kind":"none"}`, "source.jdbcUrl"},
		{"extension url setting", `{"kind":"custom","type":"mix","settings":{"endpoint":"http://localhost:9000/feed","account":"x"}}`, `{"kind":"none"}`, "source.settings.endpoint"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parse(t, definition(c.source, c.split)).Validate()
			assert.Contains(t, fields(p), c.field, "%v", p)
		})
	}
	for _, ok := range []string{
		dbPoll("jdbc:postgresql://10.0.0.5:5432/orders"),
		dbPoll("jdbc:postgresql://db1.example.com,db2.example.com:5433/orders?ssl=true"),
		dbPoll("jdbc:sqlserver://sql.corp\\INST01:1433;databaseName=orders"),
		dbPoll("jdbc:oracle:thin:@//ora.corp:1521/ORDERS"),
		dbPoll("jdbc:oracle:thin:@ora.corp:1521:ORDERS"),
		dbPoll("jdbc:mysql://[fd12::5]:3306/orders"),
		`{"kind":"sftp","host":"192.168.10.4","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"}`,
	} {
		assert.Empty(t, parse(t, definition(ok, `{"kind":"none"}`)).Validate(), ok)
	}
	// A saved definition is held to the guard when it is tested.
	saved := parse(t, definition(`{"kind":"sftp","host":"localhost","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"}`, `{"kind":"none"}`))
	assert.Equal(t, []string{"source.host"}, fields(ingestion.CheckSourceAddresses(saved.Source)))
}

func apiPoll(url, auth string) string {
	return `{"kind":"api-poll","url":"` + url + `","method":"GET","auth":` + auth + `,"results":{"kind":"json-path","path":"$"},"every":"PT1M","idleDelay":"PT5M"}`
}

func dbPoll(jdbc string) string {
	return `{"kind":"database-poll","jdbcUrl":"` + strings.ReplaceAll(jdbc, `\`, `\\`) + `","username":"u","password":{"name":"pw"},"table":"orders","track":{"kind":"watermark","column":"id","type":"NUMBER"},"maxRows":100,"every":"PT1M"}`
}

// What saving a definition asks of the caller beyond create or update (owner
// rulings R2, R4a, R4d), for the built-in kinds (which read no runtime
// report, so no repository is needed).
func TestCheckAuthor(t *testing.T) {
	ctx := context.Background()
	var repo *ingestion.Repository
	who := func(perms ...string) *auth.AuthContext {
		return &auth.AuthContext{PrincipalID: "prn_author000001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: perms}
	}
	code := func(err error) string {
		if ue := usecase.AsError(err); ue != nil {
			return ue.Code
		}
		return ""
	}
	sqs := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"orders"}`, `{"kind":"none"}`))
	sqsKeyed := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"orders","credential":{"accessKeyId":"AKIAORDERS","secretAccessKey":{"name":"aws-key"}}}`, `{"kind":"none"}`))
	sftp := parse(t, definition(`{"kind":"sftp","host":"sftp.example.com","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"}`, `{"kind":"none"}`))

	// R4a: the runtime's own AWS identity.
	assert.Equal(t, "INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Next: sqs})))
	require.NoError(t, repo.CheckAuthor(ctx, who(auth.PermIngestionRuntimeIdentity), ingestion.Change{Next: sqs}))
	renamed := sqs
	renamed.Name = "Renamed"
	require.NoError(t, repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sqs, Live: &sqs, Next: renamed}),
		"the live source already uses the identity: a change elsewhere needs nothing more")
	otherQueue := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"payments"}`, `{"kind":"none"}`))
	assert.Equal(t, "INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sqs, Live: &sqs, Next: otherQueue})),
		"pointing the identity at another queue")
	assert.Equal(t, "INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sqs, Next: renamed})),
		"nothing live yet: as a create")

	// R4d: naming secrets.
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Next: sftp})), "a create that names a secret")
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Next: sqsKeyed})))
	require.NoError(t, repo.CheckAuthor(ctx, who(auth.PermIngestionBindSecrets), ingestion.Change{Next: sftp}))
	sameHost := sftp
	sameHost.Name = "Renamed"
	sameHost.Source = func() ingestion.Source { s := sftp.Source.(ingestion.Sftp); s.Directory = "/in/new"; return s }()
	require.NoError(t, repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sftp, Live: &sftp, Next: sameHost}),
		"same host, same secrets: a directory or a name changes nothing the secret is sent to")
	otherHost := sftp
	otherHost.Source = func() ingestion.Source { s := sftp.Source.(ingestion.Sftp); s.Host = "evil.example.net"; return s }()
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sftp, Live: &sftp, Next: otherHost})),
		"the same secret sent to another host")
	otherSecret := sftp
	otherSecret.Source = func() ingestion.Source {
		s := sftp.Source.(ingestion.Sftp)
		s.Credential.Name = "payroll-password"
		return s
	}()
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", code(repo.CheckAuthor(ctx, who(), ingestion.Change{Current: &sftp, Live: &sftp, Next: otherSecret})),
		"another secret sent to the same host")
	noSecret := parse(t, definition(`{"kind":"http","auth":{"kind":"flowcatalyst-token","application":"orders"}}`, `{"kind":"none"}`))
	require.NoError(t, repo.CheckAuthor(ctx, who(), ingestion.Change{Next: noSecret}), "naming no secret needs neither")
}

// An extension kind's settings all count as secret names, so any change of
// them points secrets somewhere new.
func TestSteersSecrets_ExtensionSettings(t *testing.T) {
	a := parse(t, definition(`{"kind":"custom","type":"mix","settings":{"account":"acme","password":"mix-password"}}`, `{"kind":"none"}`))
	b := parse(t, definition(`{"kind":"custom","type":"mix","settings":{"account":"other","password":"mix-password"}}`, `{"kind":"none"}`))
	assert.False(t, ingestion.SteersSecrets(a, a))
	assert.True(t, ingestion.SteersSecrets(a, b))
	c := parse(t, definition(`{"kind":"custom","type":"webiq","settings":{"account":"acme","password":"mix-password"}}`, `{"kind":"none"}`))
	assert.True(t, ingestion.SteersSecrets(a, c), "another kind")
}

// The route permission counts only at anchor tier (owner ruling R2); any
// other declared permission counts at any tier.
func TestRequireKindPermission_RouteNeedsAnchor(t *testing.T) {
	anchor := &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeService, Permissions: []string{auth.PermIngestionRoute, "x:y:z:w"}}
	client := &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_1"}, PrincipalType: auth.PrincipalTypeService, Permissions: []string{auth.PermIngestionRoute, "x:y:z:w"}}
	require.NoError(t, ingestion.RequireKindPermission(anchor, "camel-route", auth.PermIngestionRoute))
	err := ingestion.RequireKindPermission(client, "camel-route", auth.PermIngestionRoute)
	require.Error(t, err)
	assert.Equal(t, "INGESTION_ROUTE_ANCHOR_REQUIRED", usecase.AsError(err).Code)
	assert.False(t, ingestion.HoldsKindPermission(client, auth.PermIngestionRoute))
	require.NoError(t, ingestion.RequireKindPermission(client, "mix", "x:y:z:w"), "an extension's own permission is not tier-bound")
	assert.Equal(t, "INGESTION_KIND_PERMISSION_REQUIRED", usecase.AsError(ingestion.RequireKindPermission(&auth.AuthContext{Scope: auth.ScopeAnchor}, "camel-route", auth.PermIngestionRoute)).Code)
	require.NoError(t, ingestion.RequireKindPermission(client, "plain", ""))
}

// Camel routes are written by a pipeline or an ingestion administrator (owner
// ruling 2026-10-25): at anchor tier with the route permission, a SERVICE
// principal (the pipeline's account holding the route-author role) may write
// one, and so may an administrator (who holds delete); a user who holds the
// route permission without being an administrator may not, nor may an
// operator, who holds no route.
func TestRequireKindPermission_RoutesArePipelineOrAdmin(t *testing.T) {
	const (
		del     = "platform:ingestion:ingestion:delete"
		create  = "platform:ingestion:ingestion:create"
		update  = "platform:ingestion:ingestion:update"
		view    = "platform:ingestion:ingestion:view"
		test    = "platform:ingestion:ingestion:test"
		secrets = "platform:ingestion:ingestion:bind-secrets"
	)
	routeAuthor := []string{view, create, update, auth.PermIngestionRoute, secrets}
	cases := []struct {
		name  string
		ac    *auth.AuthContext
		wants string // "" allowed
	}{
		{"a user with the route-author role", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: routeAuthor}, "INGESTION_ROUTE_PIPELINE_REQUIRED"},
		{"a user with route only", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: []string{auth.PermIngestionRoute}}, "INGESTION_ROUTE_PIPELINE_REQUIRED"},
		{"a pipeline's service account with the route-author role", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeService, Permissions: routeAuthor}, ""},
		{"an ingestion administrator (a user)", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: append([]string{del, "platform:ingestion:ingestion:promote"}, routeAuthor...)}, ""},
		{"super-admin's wildcard", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: []string{"platform:*:*:*"}}, ""},
		{"an operator", &auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: []string{view, update, test}}, "INGESTION_KIND_PERMISSION_REQUIRED"},
		{"a pipeline below anchor tier", &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_1"}, PrincipalType: auth.PrincipalTypeService, Permissions: routeAuthor}, "INGESTION_ROUTE_ANCHOR_REQUIRED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ingestion.RequireKindPermission(c.ac, "camel-route", auth.PermIngestionRoute)
			if c.wants == "" {
				require.NoError(t, err)
				assert.True(t, ingestion.HoldsKindPermission(c.ac, auth.PermIngestionRoute))
				return
			}
			require.Error(t, err)
			assert.Equal(t, c.wants, usecase.AsError(err).Code)
			assert.False(t, ingestion.HoldsKindPermission(c.ac, auth.PermIngestionRoute))
		})
	}
	// Another kind's declared permission is not a route: a user may hold it.
	require.NoError(t, ingestion.RequireKindPermission(&auth.AuthContext{Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser, Permissions: []string{"x:y:z:w"}}, "mix", "x:y:z:w"))
}

// builtinKinds (C1): a runtime that declares them carries exactly those
// built-ins and its extension kinds; one that does not carries the v2
// baseline, or (the v2 way) only the built-ins it names among its kinds.
func TestCarriedKinds(t *testing.T) {
	ext := []ingestion.RuntimeKind{{Kind: "mix"}, {Kind: "camel-route"}}
	assert.ElementsMatch(t, append([]string{"mix", "camel-route"}, ingestion.BaselineBuiltInKinds()...), ingestion.CarriedKinds(ext, nil))
	assert.ElementsMatch(t, []string{"mix", "camel-route", "sqs", "http"}, ingestion.CarriedKinds(ext, []string{"sqs", "http", "custom", "ftp"}),
		"declared built-ins only; custom and unknown names carry nothing")
	assert.ElementsMatch(t, []string{"mix", "camel-route"}, ingestion.CarriedKinds(ext, []string{}), "an empty declaration: extensions only")
	assert.ElementsMatch(t, []string{"mix", "sqs"}, ingestion.CarriedKinds([]ingestion.RuntimeKind{{Kind: "sqs"}, {Kind: "mix"}}, nil), "v2: naming built-ins narrows")
	assert.ElementsMatch(t, []string{"mix", "sftp"}, ingestion.CarriedKinds([]ingestion.RuntimeKind{{Kind: "sqs"}, {Kind: "mix"}}, []string{"sftp"}),
		"a declaration wins over built-ins named among the kinds")
}

// An sftp source names the server's host key or accepts any (the runtime
// refuses one that does neither, which is a warning, not a refusal, so a
// definition saved before still saves); accepting any widens where the
// credential goes and needs bind-secrets when it is turned on.
func TestSftpHostKey(t *testing.T) {
	ctx := context.Background()
	var repo *ingestion.Repository
	src := func(extra string) ingestion.Definition {
		return parse(t, definition(`{"kind":"sftp","host":"sftp.example.com","port":22,"directory":"/in","username":"u","credential":{"name":"pw"},"every":"PT5M"`+extra+`}`, `{"kind":"none"}`))
	}
	neither, keyed, anyKey := src(""), src(`,"knownHosts":"sftp.example.com ssh-ed25519 AAAA"`), src(`,"acceptAnyHostKey":true`)
	assert.Empty(t, neither.Validate())
	assert.Len(t, neither.Warnings(), 1)
	assert.Contains(t, neither.Warnings()[0], "source.knownHosts")
	assert.Empty(t, keyed.Warnings())
	assert.Empty(t, anyKey.Warnings())
	assert.Equal(t, []string{"source.acceptAnyHostKey"}, fields(src(`,"knownHosts":"h k","acceptAnyHostKey":true`).Validate()))

	secrets := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{auth.PermIngestionBindSecrets}}
	none := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	err := repo.CheckAuthor(ctx, none, ingestion.Change{Current: &keyed, Live: &keyed, Next: anyKey})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepting any sftp host key")
	require.NoError(t, repo.CheckAuthor(ctx, secrets, ingestion.Change{Current: &keyed, Live: &keyed, Next: anyKey}))
	renamed := anyKey
	renamed.Name = "Renamed"
	require.NoError(t, repo.CheckAuthor(ctx, none, ingestion.Change{Current: &anyKey, Live: &anyKey, Next: renamed}), "already on: nothing more")
}

func TestStripNUL(t *testing.T) {
	assert.Equal(t, "ab", ingestion.StripNUL("a\x00b"))
	assert.Equal(t, "plain", ingestion.StripNUL("plain"))
	s := "x\x00y"
	assert.Equal(t, "xy", *ingestion.StripNULPtr(&s))
	assert.Nil(t, ingestion.StripNULPtr(nil))
}

func TestUsesRuntimeIdentity(t *testing.T) {
	assert.True(t, ingestion.UsesRuntimeIdentity(ingestion.Sqs{Region: "eu-west-1", Queue: "q"}))
	assert.True(t, ingestion.UsesRuntimeIdentity(ingestion.S3{Region: "eu-west-1", Bucket: "b"}))
	assert.False(t, ingestion.UsesRuntimeIdentity(ingestion.S3{Credential: &ingestion.AWSCredential{AccessKeyID: "AKIA1234"}}))
	assert.False(t, ingestion.UsesRuntimeIdentity(ingestion.Sftp{}))
}
