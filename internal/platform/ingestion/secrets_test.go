package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every secret a source can name is found by walking the typed model, and
// nothing else is: a non-secret setting that looks like a name is not.
func TestSecretNames(t *testing.T) {
	ref := func(name string) SecretRef { return SecretRef{Name: name} }
	cases := []struct {
		name string
		src  Source
		want []string
	}{
		{"sftp", Sftp{Host: "sftp.example.com", Username: "feed", Credential: ref("sftp-password")}, []string{"sftp-password"}},
		{"s3 key", S3{Bucket: "b", Credential: &AWSCredential{AccessKeyID: "AKIA", SecretAccessKey: ref("s3-secret")}}, []string{"s3-secret"}},
		{"s3 own identity", S3{Bucket: "b"}, nil},
		{"sqs key", Sqs{Queue: "q", Credential: &AWSCredential{SecretAccessKey: ref("sqs-secret")}}, []string{"sqs-secret"}},
		{"sqs own identity", Sqs{Queue: "q"}, nil},
		{"rabbitmq", RabbitMQ{Username: "u", Password: ref("mq-password")}, []string{"mq-password"}},
		{"database", DatabasePoll{Username: "u", Password: ref("db-password")}, []string{"db-password"}},
		{"http basic", HTTP{Auth: BasicAuth{Username: "u", Password: ref("in-password")}}, []string{"in-password"}},
		{"http bearer", HTTP{Auth: BearerAuth{Token: ref("in-token")}}, []string{"in-token"}},
		{"http platform token", HTTP{Auth: FlowCatalystToken{Application: "acme"}}, nil},
		{"api none", APIPoll{Auth: NoAuth{}}, nil},
		{"api basic", APIPoll{Auth: APIBasic{Username: "u", Password: ref("api-password")}}, []string{"api-password"}},
		{"api bearer", APIPoll{Auth: APIBearer{Token: ref("api-token")}}, []string{"api-token"}},
		{"api key", APIPoll{Auth: APIKey{Header: "X-Key", Value: ref("api-key")}}, []string{"api-key"}},
		{"api oauth", APIPoll{Auth: ClientCredentials{ClientID: "id", ClientSecret: ref("oauth-secret")}}, []string{"oauth-secret"}},
		{"api login", APIPoll{Auth: Login{Username: "u", Password: ref("login-password")}}, []string{"login-password"}},
		{"custom text settings", Custom{Type: "mix", Settings: map[string]any{
			"password": "mix-password", "retries": float64(3), "on": true, "names": []any{"a-secret", "b-secret"}, "empty": "",
		}}, []string{"a-secret", "b-secret", "mix-password"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ElementsMatch(t, tc.want, SecretNames(tc.src))
		})
	}
}

// The names are read from a definition as the platform stores it, not from a
// rendering of it: a definition file's source gives the same answer.
func TestSecretNames_FromADefinition(t *testing.T) {
	src, err := UnmarshalSource([]byte(`{"kind":"api-poll","url":"https://api.example.com/orders","method":"GET",
		"auth":{"kind":"oauth-client-credentials","tokenUrl":"https://idp.example.com/token","clientId":"id","clientSecret":{"name":"oauth-secret"},"credentialsIn":"BODY"},
		"results":{"kind":"records"},"every":"PT5M","idleDelay":"PT10M"}`))
	assert.NoError(t, err)
	assert.Equal(t, []string{"oauth-secret"}, SecretNames(src))
}
