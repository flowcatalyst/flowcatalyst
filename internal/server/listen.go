package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// BindLoopback is the FC_API_BIND value that listens on the local machine only.
const BindLoopback = "loopback"

// listenAddrs maps (bind, port) to the addresses the API and metrics
// listeners open. Empty bind: every interface (today's behaviour).
func listenAddrs(bind string, port int) []string {
	p := strconv.Itoa(port)
	switch bind {
	case "":
		return []string{":" + p}
	case BindLoopback:
		return []string{net.JoinHostPort("127.0.0.1", p), net.JoinHostPort("::1", p)}
	default:
		return []string{net.JoinHostPort(bind, p)}
	}
}

// openListeners binds every address for (bind, port). The first address must
// bind; for loopback the second (IPv6) is best effort, since a host may have no
// IPv6 loopback. With port 0 the IPv6 listener reuses the port the IPv4 one got.
func openListeners(bind string, port int) ([]net.Listener, error) {
	addrs := listenAddrs(bind, port)
	first, err := net.Listen("tcp", addrs[0])
	if err != nil {
		return nil, err
	}
	lns := []net.Listener{first}
	for _, a := range addrs[1:] {
		if port == 0 {
			if tcp, ok := first.Addr().(*net.TCPAddr); ok {
				a = net.JoinHostPort("::1", strconv.Itoa(tcp.Port))
			}
		}
		ln, err := net.Listen("tcp", a)
		if err != nil {
			slog.Debug("listener unavailable; continuing without it", "addr", a, "err", err)
			continue
		}
		lns = append(lns, ln)
	}
	return lns, nil
}

func closeAll(lns []net.Listener) {
	for _, ln := range lns {
		_ = ln.Close()
	}
}

// serveAll serves srv on every listener; a failure other than a normal close
// is sent to errc (which must have room for len(lns) errors).
func serveAll(srv *http.Server, lns []net.Listener, name string, errc chan<- error) {
	for _, ln := range lns {
		go func() {
			slog.Info(name+" server listening", "addr", ln.Addr().String())
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("%s server: %w", name, err)
			}
		}()
	}
}

// useEdgeMiddleware mounts the outermost middleware in the order the server
// relies on: the peer recorder must see the connection's real RemoteAddr, so it
// goes before RealIP (which rewrites RemoteAddr from client-supplied headers).
func useEdgeMiddleware(r chi.Router, realIP bool) {
	r.Use(platformmw.RecordPeerAddr)
	r.Use(middleware.RequestID)
	if realIP {
		// chi RealIP rewrites RemoteAddr from True-Client-IP, then X-Real-IP,
		// then the LEFTMOST X-Forwarded-For hop, all client-controlled
		// (GO-2026-5774/5775/5777). chi 5.3.0 only DEPRECATED it (behaviour
		// unchanged, still spoofable); upstream points at the ClientIPFrom*
		// helpers, which are not adopted here.
		// Kept on by default for compatibility; the platform's own rate limits
		// key on the rightmost XFF hop (ratelimit.ClientIP) and fall back to
		// RemoteAddr only when XFF is absent.
		r.Use(middleware.RealIP) //nolint:staticcheck // SA1019: deprecated in chi 5.3.0, behaviour kept deliberately
	}
}
