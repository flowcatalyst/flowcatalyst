// Package netguard keeps outbound deliveries away from addresses that should
// never be a customer webhook: the machine's own loopback, cloud metadata
// (169.254.169.254 and its IPv6 sibling), link-local ranges, and — unless
// allowed — private networks such as the cluster's own pod and service
// addresses.
//
// A subscription endpoint is attacker-influenced input: whoever can create a
// subscription chooses a URL the platform will POST to, from inside the
// cluster, carrying the platform's credentials. Without a guard that is a
// server-side request forgery primitive against whatever the cluster can reach.
//
// Two checks, because neither is enough alone:
//
//   - ValidateURL rejects a bad URL when it is written, so the operator gets an
//     immediate, readable error instead of a delivery that fails forever.
//   - DialContext checks the address actually being connected to, after DNS
//     resolution. This is the authoritative check: a hostname can resolve to a
//     private address (or be changed to one after validation), and only the
//     dial sees the final answer. It also covers every redirect and retry,
//     since each is a fresh connection.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
)

// Policy decides which destinations outbound deliveries may reach. The zero
// value is the strict policy: public addresses only.
type Policy struct {
	// AllowLoopback permits 127.0.0.0/8, ::1 and the name "localhost". For
	// development, where webhook receivers run on the same machine.
	AllowLoopback bool
	// AllowPrivate permits RFC 1918, unique-local (fc00::/7) and other
	// non-public unicast ranges. Off by default: in a cluster these are the
	// pods, services and internal APIs a forged request could reach.
	AllowPrivate bool

	allowHosts []string // host:port patterns; * matches within a label set
}

// Default is the policy the platform's delivery paths consult. It starts from
// the environment (FromEnv) so every binary — server, standalone router, dev
// tool — is strict unless told otherwise; the server adds the platform's own
// internal endpoints at startup.
var Default = FromEnv()

// FromEnv reads the delivery policy:
//
//	FC_DELIVERY_ALLOW_LOOPBACK  permit loopback destinations (dev only)
//	FC_DELIVERY_ALLOW_PRIVATE   permit private-network destinations
//	FC_DELIVERY_ALLOW_HOSTS     comma-separated host:port patterns exempt from
//	                            the checks, e.g. "runner.internal:8095,*.fn.svc:8095"
//
// Unset or unparseable values leave the strict default.
func FromEnv() *Policy {
	p := &Policy{}
	p.AllowLoopback, _ = strconv.ParseBool(os.Getenv("FC_DELIVERY_ALLOW_LOOPBACK"))
	p.AllowPrivate, _ = strconv.ParseBool(os.Getenv("FC_DELIVERY_ALLOW_PRIVATE"))
	for h := range strings.SplitSeq(os.Getenv("FC_DELIVERY_ALLOW_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			p.AllowHost(h)
		}
	}
	return p
}

// AllowHost exempts a host:port pattern (lower-cased; * is a wildcard, as in
// path.Match) from the address checks. For the platform's own internal
// endpoints — the dispatch processing callback, the function runner — which
// legitimately live on a loopback or private address. Not for customer input.
func (p *Policy) AllowHost(hostPort string) {
	p.allowHosts = append(p.allowHosts, strings.ToLower(hostPort))
}

// AllowURL is AllowHost for a URL's authority, filling in the scheme's default
// port. A "{pool}" placeholder (the function runner's URL template) becomes a
// wildcard. Unparseable input is ignored.
func (p *Policy) AllowURL(raw string) {
	u, err := url.Parse(strings.ReplaceAll(raw, "{pool}", "wildcard-pool"))
	if err == nil && strings.Contains(raw, "{pool}") {
		u.Host = strings.ReplaceAll(u.Host, "wildcard-pool", "*")
	}
	if err != nil || u.Hostname() == "" {
		return
	}
	p.AllowHost(hostPortOf(u))
}

func hostPortOf(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// ErrBlocked is wrapped by every rejection so callers can tell "the policy
// said no" from a network failure.
var ErrBlocked = errors.New("destination not allowed")

// metadataV6 is the AWS IPv6 instance-metadata address; it sits inside
// fc00::/7, so allowing private ranges must not let it through.
var metadataV6 = netip.MustParseAddr("fd00:ec2::254")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// CheckIP reports why ip may not be dialled, or nil.
func (p *Policy) CheckIP(ip netip.Addr) error {
	ip = ip.Unmap()
	switch {
	case !ip.IsValid():
		return fmt.Errorf("%w: invalid address", ErrBlocked)
	case ip.IsUnspecified():
		return fmt.Errorf("%w: %s is the unspecified address", ErrBlocked, ip)
	case ip.IsMulticast() || ip.IsInterfaceLocalMulticast():
		return fmt.Errorf("%w: %s is a multicast address", ErrBlocked, ip)
	case ip.IsLinkLocalUnicast():
		// 169.254.0.0/16 — includes the cloud metadata service — and fe80::/10.
		return fmt.Errorf("%w: %s is a link-local address", ErrBlocked, ip)
	case ip == metadataV6:
		return fmt.Errorf("%w: %s is the cloud metadata address", ErrBlocked, ip)
	case ip.IsLoopback():
		if !p.AllowLoopback {
			return fmt.Errorf("%w: %s is a loopback address", ErrBlocked, ip)
		}
		return nil
	case ip.IsPrivate() || cgnat.Contains(ip) || !ip.IsGlobalUnicast():
		if !p.AllowPrivate {
			return fmt.Errorf("%w: %s is a private or reserved address", ErrBlocked, ip)
		}
	}
	return nil
}

// ValidateURL is the write-time check for a delivery URL: an absolute http or
// https URL, without embedded credentials, whose host is not an address (or
// the name "localhost") the policy forbids. Host names are resolved and
// checked at dial time; a name that merely looks harmless passes here.
func (p *Policy) ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("must be an http or https URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return errors.New("must include a host")
	}
	if u.User != nil {
		return errors.New("must not embed credentials")
	}
	if p.hostAllowed(hostPortOf(u)) {
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		if !p.AllowLoopback {
			return fmt.Errorf("%w: %s is a loopback name", ErrBlocked, host)
		}
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return p.CheckIP(ip)
	}
	return nil
}

func (p *Policy) hostAllowed(hostPort string) bool {
	hostPort = strings.ToLower(hostPort)
	for _, pat := range p.allowHosts {
		if ok, _ := path.Match(pat, hostPort); ok {
			return true
		}
	}
	return false
}

// Control is a net.Dialer.Control hook: it runs after DNS resolution, on the
// exact address about to be connected, so it sees what the resolver said and
// not what the URL claimed.
func (p *Policy) Control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address %q", ErrBlocked, address)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: unparseable address %q", ErrBlocked, address)
	}
	return p.CheckIP(ip)
}

// DialContext returns a dial function for an http.Transport that refuses
// forbidden destinations. Hosts exempted with AllowHost skip the check.
// base's own Control hook, if any, is replaced for guarded dials.
func (p *Policy) DialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	guarded := *base
	guarded.Control = p.Control
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if p.hostAllowed(addr) {
			return base.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
}
