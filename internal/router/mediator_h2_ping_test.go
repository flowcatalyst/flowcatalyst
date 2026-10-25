package router

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// stalledH2Peer is an HTTP/2 server that completes the connection preface and
// SETTINGS exchange and then goes silent: it never answers a request and never
// acks a PING. To the client that is indistinguishable from a half-open
// connection (a peer or middlebox that vanished without a FIN or RST) — no
// frame will ever arrive again.
type stalledH2Peer struct {
	addr  string
	pings atomic.Int32
	mu    sync.Mutex
	conns []net.Conn
}

func startStalledH2Peer(t *testing.T, tlsCfg *tls.Config) *stalledH2Peer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	p := &stalledH2Peer{addr: ln.Addr().String()}
	t.Cleanup(func() {
		_ = ln.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			p.conns = append(p.conns, c)
			p.mu.Unlock()
			go p.serve(c)
		}
	}()
	return p
}

func (p *stalledH2Peer) serve(c net.Conn) {
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(c, preface); err != nil || !bytes.Equal(preface, []byte(http2.ClientPreface)) {
		return
	}
	fr := http2.NewFramer(c, c)
	if fr.WriteSettings() != nil {
		return
	}
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			return
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() && fr.WriteSettingsAck() != nil {
				return
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				p.pings.Add(1) // seen, deliberately never acked
			}
		}
		// HEADERS/DATA for the request: swallowed, never answered.
	}
}

func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}
}

// deliverTo POSTs to url through a client from the mediator's own builder and
// reports the error and how long it took.
func deliverTo(t *testing.T, cfg MediatorConfig, url string, deadline time.Duration) (time.Duration, error) {
	t.Helper()
	client := newClientBuilder(cfg)()
	if rt, ok := client.Transport.(*schemeRoundTripper); ok {
		if tr, ok := rt.other.(*http.Transport); ok {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // G402: test peer's self-signed cert
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(`{"messageId":"m"}`)))
	require.NoError(t, err)
	start := time.Now()
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return time.Since(start), err
}

// TestHTTP2HealthCheckClosesAStalledConnection pins the HTTP/2 PING health
// check on both delivery transports (h2c for http://, ALPN h2 for https://).
// A connection that stops delivering frames must be pinged after
// HTTP2ReadIdleTimeout and closed when the ping goes unanswered, failing the
// request promptly — not left to the request's own deadline, which in
// production is the 15-minute processing contract.
func TestHTTP2HealthCheckClosesAStalledConnection(t *testing.T) {
	healthChecked := DefaultMediatorConfig()
	healthChecked.HTTP2ReadIdleTimeout = 200 * time.Millisecond
	healthChecked.HTTP2PingTimeout = 300 * time.Millisecond

	for _, tc := range []struct {
		name   string
		tls    bool
		scheme string
	}{
		{"h2c (http)", false, "http"},
		{"h2 over TLS (https)", true, "https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tlsCfg *tls.Config
			if tc.tls {
				tlsCfg = selfSignedTLS(t)
			}
			peer := startStalledH2Peer(t, tlsCfg)

			took, err := deliverTo(t, healthChecked, tc.scheme+"://"+peer.addr+"/deliver", 10*time.Second)
			require.Error(t, err, "nothing answers; the request must fail")
			assert.NotErrorIs(t, err, context.DeadlineExceeded,
				"the health check, not the request deadline, must end it")
			assert.Less(t, took, 5*time.Second, "the stalled connection must be detected in ~read-idle + ping timeout")
			assert.Positive(t, peer.pings.Load(), "the client must have pinged the silent connection")
		})
	}
}

// TestHTTP2WithoutHealthCheckWaitsForTheDeadline is the control for the test
// above: with the health check off (zero timeouts, the x/net default) the same
// stalled peer is never pinged and the request runs to its own deadline. It
// shows the stalled peer really does stall a client, so the test above
// measures the health check and nothing else.
func TestHTTP2WithoutHealthCheckWaitsForTheDeadline(t *testing.T) {
	off := DefaultMediatorConfig()
	off.HTTP2ReadIdleTimeout = 0
	off.HTTP2PingTimeout = 0
	peer := startStalledH2Peer(t, nil)

	// Sample the pings while the request is still waiting: once the deadline
	// cancels it, x/net's transport sends one PING to learn whether the
	// connection survived the reset (not a health check), so counting after
	// the deadline would race with that.
	var beforeDeadline atomic.Int32
	beforeDeadline.Store(-1)
	sample := time.AfterFunc(1300*time.Millisecond, func() { beforeDeadline.Store(peer.pings.Load()) })
	defer sample.Stop()

	took, err := deliverTo(t, off, "http://"+peer.addr+"/deliver", 1500*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.GreaterOrEqual(t, took, 1400*time.Millisecond)
	assert.Zero(t, beforeDeadline.Load(), "no health-check ping while the request waits")
}

// TestMediatorHTTP2HealthCheckDefaultsAndOverrides pins the defaults and the
// ServerConfig overrides (FC_ROUTER_HTTP2_READ_IDLE_TIMEOUT_SECONDS /
// FC_ROUTER_HTTP2_PING_TIMEOUT_SECONDS).
func TestMediatorHTTP2HealthCheckDefaultsAndOverrides(t *testing.T) {
	def := DefaultMediatorConfig()
	assert.Equal(t, 30*time.Second, def.HTTP2ReadIdleTimeout)
	assert.Equal(t, 15*time.Second, def.HTTP2PingTimeout)

	got := mediatorConfigFor(ServerConfig{})
	assert.Equal(t, def.HTTP2ReadIdleTimeout, got.HTTP2ReadIdleTimeout, "zero keeps the default")
	assert.Equal(t, def.HTTP2PingTimeout, got.HTTP2PingTimeout, "zero keeps the default")
	assert.Equal(t, def.Timeout, got.Timeout)

	got = mediatorConfigFor(ServerConfig{HTTP2ReadIdleTimeout: 10 * time.Second, HTTP2PingTimeout: 5 * time.Second})
	assert.Equal(t, 10*time.Second, got.HTTP2ReadIdleTimeout)
	assert.Equal(t, 5*time.Second, got.HTTP2PingTimeout)

	dev := mediatorConfigFor(ServerConfig{DevMode: true})
	assert.Equal(t, DevMediatorConfig().Timeout, dev.Timeout, "dev mode still selects the dev mediator")
}
