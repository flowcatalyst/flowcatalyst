package pushgw

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// maxScopedBody bounds the subscribe body fieldScope reads; a larger one is passed on untouched
// (the module refuses it).
const maxScopedBody = 1 << 20

// fieldScope narrows a field worker's tickets to the application their token is for (spec
// section 5, owner ruling 2026-11-09): a worker signed in to application A may be granted A's
// topics only, by rule or by ticket. Rules are narrowed inside the module's decision with the
// synthetic permission [AppAccessPrefix]; a ticket's grant is the module's own, so the tickets
// that name another application as their issuer are removed from the subscribe before the module
// sees it. Their topics are then denied (TOPIC_FORBIDDEN) like any topic nothing grants.
//
// The token's application and a ticket's issuer are read without verifying: the filter only ever
// removes tickets, and a token that does not verify is refused by the module whatever the filter
// did. Only POST .../subscriptions with Authorization: DPoP is touched.
func fieldScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/push/subscriptions") {
			if scheme, token := authorizationScheme(r); strings.EqualFold(scheme, "DPoP") {
				narrowTickets(r, tokenAudience(token))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// narrowTickets removes from the request body the tickets whose issuer is not aud. It leaves the
// request as it was whenever the body is not what it expects.
func narrowTickets(r *http.Request, aud string) {
	if aud == "" || r.Body == nil {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxScopedBody+1))
	restore := func(b []byte) {
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), r.Body))
	}
	if err != nil || len(raw) > maxScopedBody {
		restore(raw)
		return
	}
	var body map[string]json.RawMessage
	var tickets []string
	if json.Unmarshal(raw, &body) != nil || json.Unmarshal(body["tickets"], &tickets) != nil || len(tickets) == 0 {
		restore(raw)
		return
	}
	kept := make([]string, 0, len(tickets))
	for _, t := range tickets {
		if iss, ok := ticketIssuer(t); ok && iss != aud {
			continue
		}
		kept = append(kept, t) // an unreadable ticket stays: the module answers it (400)
	}
	if len(kept) == len(tickets) {
		restore(raw)
		return
	}
	body["tickets"], _ = json.Marshal(kept)
	out, err := json.Marshal(body)
	if err != nil {
		restore(raw)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(out))
	r.ContentLength = int64(len(out))
	r.Header.Set("Content-Length", strconv.Itoa(len(out)))
}

// tokenAudience is the aud claim of a field access token, unverified: the application code.
func tokenAudience(token string) string {
	var c struct {
		Aud string `json:"aud"`
	}
	if !jwtClaims(token, &c) {
		return ""
	}
	return c.Aud
}

// ticketIssuer is the iss claim of a ticket, unverified.
func ticketIssuer(ticket string) (string, bool) {
	var c struct {
		Iss string `json:"iss"`
	}
	if !jwtClaims(ticket, &c) || c.Iss == "" {
		return "", false
	}
	return c.Iss, true
}

func jwtClaims(compact string, into any) bool {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	return err == nil && json.Unmarshal(b, into) == nil
}
