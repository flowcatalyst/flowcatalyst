package scheduler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// Both delivery clients (customer policy and internal policy) must refuse
// to follow a redirect: the signed body and signature headers would otherwise
// be re-sent to the redirect target.
func TestDeliveryClients_DoNotFollowRedirects(t *testing.T) {
	prev := netguard.Internal.AllowLoopback
	netguard.Internal.AllowLoopback = true
	t.Cleanup(func() { netguard.Internal.AllowLoopback = prev })

	svc := NewService(Config{}, nil, nil, nil, nil)
	clients := map[string]*http.Client{"customer": svc.dispatcher.http, "function": svc.dispatcher.fnHTTP}

	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			var second atomic.Int32
			elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				second.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer elsewhere.Close()
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/direct" {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
			}))
			defer first.Close()

			req, err := http.NewRequest(http.MethodPost, first.URL+"/redir", strings.NewReader("signed-body"))
			require.NoError(t, err)
			req.Header.Set("X-FlowCatalyst-Signature", "sig")
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "the 3xx is returned, not followed")
			assert.EqualValues(t, 0, second.Load(), "the redirect target must receive no request")

			req, err = http.NewRequest(http.MethodPost, first.URL+"/direct", strings.NewReader("x"))
			require.NoError(t, err)
			resp, err = client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
