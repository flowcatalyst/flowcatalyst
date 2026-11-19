package sessiontoken_test

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

// fuzzKeyPEM is a key for this fuzz target only, fixed so that every fuzzing
// process (the coordinator and each worker) verifies with the same key: with a
// key generated per process a worker could never verify a corpus entry, and
// fuzzing would explore nothing near a valid token. It signs nothing else.
const fuzzKeyPEM = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQD5olA9OxNFFBz1
iDpAGX/XlwM/j94i9e/ZbeC2ipdTHI5lvMuVSiS/2gbgeSJlMIbjtF2fgDEEtAAG
+HXEahmEFc6B6nOCpNgW59Ro4lmzRyIwf6qNbG2E6SBCIY7q+oppt0JQNaYPB0AQ
bQtA13W75benXoQSA2SVB/g7ST0J+U07BNbGr00TL0/lEZbUSXtkdq5w5/I1xkis
eijlPugafRcTuUmXuN2fZwRm+vyrqONn1jgJl/HfmIGkvUJcbnyyEYDQlG49YT8K
ZAvZK6xf7XMcV95vZfabzH0D9PG8M4JikP+8HwmyF5G23UVsPqPlR86dsQ08F8Bz
NlOnFt7pAgMBAAECggEAGD2mxQtjh6W4Wi9sWKUhQPItIpL54zM2o0qEM7WplCyF
t2Np8YlNoEOvv0SEDodz0dDF9qMgH3DLP+IIrZYP/D+R2hPGO71+Lls8ySKRy1Lx
AvyitzD8QZfRxho0IqVJo5JkIh3TjkoJ8o5kNVu21xQrjVkkQguKTN6FQFyQTNNz
j6e4iNOI2ucDYwxqvzvODkLFjOaRJUTdNF8jIA+Zc+pUd417BCDu3ZblPobI44ld
WMcgyj1ZNQ5Tkq1Xd3NkBkYq4KwE2zHF5oaDTFhmL6MG9X7erD2LlS8DZXE3HZLh
EusmT+7z9GQTkdZPQh1uhulWOKi3iXDqlAuKkYQ4MQKBgQD9fG6HDoDYaZY4iC+V
0rzYfolhoFPY4VR92MHO84jKUItH7vKDedQBJmlPLp0TYrtBpFJd4VCFZY1umHLf
hmAPD2tMJRNucR30A8peipl5zGW1562I6dcQJhJ7pfNGuf0+hdsrLEGpWTGwJ9i1
Lz4oAnPHmsjE4qWLuq4/fRGQJwKBgQD8HBoVxYin5KfNbKjPdhpXgu/h03hXSPB8
aqKmPtYTWXsLuqlixgq24Vj87TkWZ+RsvWGtXuaUxGZxdXxXYUkE0bTNQTClZqBA
jhDVlEFSRezYDkvgQfgjp6nkOZ4LmbRO5Gy62qJ9VilNScDHQDE2XHpbkTCumSV0
FCmFuHhybwKBgQCJ04h9YsNn6DFH17SX8BfCVzr3nCc6+3nnW1412nD8IEM6/49Z
MXq2jV4hMaUyjzJmBKTMauBPR+JYygdbxkKgra+XnWktWdABh7YXJaFBeoOrHqda
lFrdavYNyTO3txUdU8bReKTTIa5Eq8XG7kMLmhm95ElZ1CpTCVQJLUXJbQKBgBx/
ULOFUJ0py4386lgJEZEyy8rLDJ9RL13g7+G62Jf3VWFOCDeSC0QA1P5CEmHl/HwO
LCxcvdOzH6FTlmqKJKbWzYkmBUmtABBL/rsltvyFyu/hLdJ7ooDlQt75ZFQLW8ZP
K7szW4qvYpvJ6nCbYPS0H2CaWNZjmUg1vzvOTk8hAoGAZFUN8qp8HPlC+c2ySNFg
QnLy3Y29EgN+p55Bj8pBKkBdFH6QhNHY5+Tad1eFZXuJB+CuE3YQStj+ysdZSa1m
m5IshjP2z+NdYvgoPDnJNjK4CFHOUdDylmWot7wAdot3xtguKswWrR7nGmAx1pBU
UBAtTikcopWae19XKPpRkjs=
-----END PRIVATE KEY-----`

func fuzzKey(f *testing.F) *rsa.PrivateKey {
	f.Helper()
	block, _ := pem.Decode([]byte(fuzzKeyPEM))
	if block == nil {
		f.Fatal("fuzz key: bad PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		f.Fatal(err)
	}
	return k.(*rsa.PrivateKey)
}

// FuzzValidate feeds arbitrary strings to the session/access-token verifier.
// It must never panic, and nothing but a token our key signed may validate,
// and that one only as it was issued: an accepted input must be a token we
// minted (subject prn_x) and must equal the standard encoder's output for its
// own bytes, so no other spelling of a token validates (compactjws). The seeds
// include the valid token with each other last character of its signature,
// padded, and with line breaks, each of which a lenient decoder reads as the
// same signature bytes. (Each fuzzing process mints its own valid token, so
// the property is about the spelling, not equality with one process's token.)
func FuzzValidate(f *testing.F) {
	key := fuzzKey(f)
	valid, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_x", Scope: "ANCHOR"}, key, "iss", time.Hour)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add(valid[:len(valid)-2])
	head, sig := valid[:strings.LastIndexByte(valid, '.')+1], valid[strings.LastIndexByte(valid, '.')+1:]
	for _, v := range compactjwstest.NonCanonicalSpellings(sig) {
		f.Add(head + v)
	}
	f.Add(valid + "==")
	f.Add(head + sig[:10] + "\n" + sig[10:])
	f.Add(valid + "\r\n")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiJwcm5feCJ9.")
	f.Add("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcm5feCJ9.AAAA")
	f.Add("")
	f.Add("....")
	f.Fuzz(func(t *testing.T, tok string) {
		c, err := sessiontoken.Validate(tok, &key.PublicKey, sessiontoken.Expect{Issuer: "iss"})
		if err != nil {
			return
		}
		if c.Subject != "prn_x" {
			t.Fatalf("a token that is not ours validated: %q -> %+v", tok, c)
		}
		if !compactjwstest.SameAsReencoded(tok) {
			t.Fatalf("a re-spelled token validated: %q", tok)
		}
	})
}
