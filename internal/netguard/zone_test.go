package netguard

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
)

// zoned are IPv6 addresses written with a zone (an interface name after %).
// The zone is not part of the address the rules judge, and no delivery may
// name an interface, so every one is refused under every policy, including
// the ones that allow loopback and private destinations.
var zoned = []string{
	"::%lo0",                      // unspecified
	"::%eth0",                     // unspecified, another interface name
	"::1%lo0",                     // loopback
	"fe80::1%eth0",                // link-local
	"fe80::1%1",                   // link-local, numeric zone
	"fd00:ec2::254%eth0",          // the IPv6 metadata address
	"fd12::1%eth0",                // unique-local (private)
	"2606:4700:4700::1111%eth0",   // global
	"ff02::1%eth0",                // multicast
	"::ffff:127.0.0.1%lo0",        // IPv4-mapped loopback
	"::ffff:10.0.0.1%eth0",        // IPv4-mapped private
	"::ffff:169.254.169.254%eth0", // IPv4-mapped metadata
	"::ffff:8.8.8.8%eth0",         // IPv4-mapped global
}

func zonePolicies() map[string]*Policy {
	exempt := &Policy{AllowLoopback: true, AllowPrivate: true}
	exempt.AllowHost("runner.internal:8095")
	return map[string]*Policy{
		"strict":   {},
		"private":  {AllowPrivate: true},
		"loopback": {AllowLoopback: true},
		"dev":      {AllowLoopback: true, AllowPrivate: true},
		"exempt":   exempt,
	}
}

func TestCheckIPRefusesZonedAddresses(t *testing.T) {
	for _, s := range zoned {
		ip := netip.MustParseAddr(s)
		for name, p := range zonePolicies() {
			err := p.CheckIP(ip)
			if err == nil {
				t.Errorf("%s policy allowed %s", name, s)
				continue
			}
			if !errors.Is(err, ErrBlocked) {
				t.Errorf("%s policy, %s: error does not wrap ErrBlocked: %v", name, s, err)
			}
		}
	}
}

// The rules judge the address without its zone: where the bare address is
// refused, the reason is the bare address's; where it would be allowed, the
// zone is the reason.
func TestCheckIPJudgesTheAddressWithoutItsZone(t *testing.T) {
	cases := []struct {
		policy *Policy
		ip     string
		reason string
	}{
		{&Policy{}, "::%lo0", "is the unspecified address"},
		{&Policy{AllowLoopback: true, AllowPrivate: true}, "::%lo0", "is the unspecified address"},
		{&Policy{}, "::1%lo0", "is a loopback address"},
		{&Policy{AllowLoopback: true}, "::1%lo0", "names a network interface"},
		{&Policy{}, "fe80::1%eth0", "is a link-local address"},
		{&Policy{AllowPrivate: true}, "fd00:ec2::254%eth0", "is the cloud metadata address"},
		{&Policy{}, "fd12::1%eth0", "is a private or reserved address"},
		{&Policy{AllowPrivate: true}, "fd12::1%eth0", "names a network interface"},
		{&Policy{}, "2606:4700:4700::1111%eth0", "names a network interface"},
		{&Policy{}, "::ffff:169.254.169.254%eth0", "is a link-local address"},
		{&Policy{AllowLoopback: true}, "::ffff:127.0.0.1%lo0", "names a network interface"},
		{&Policy{}, "::ffff:8.8.8.8%eth0", "names a network interface"},
	}
	for _, c := range cases {
		err := c.policy.CheckIP(netip.MustParseAddr(c.ip))
		if err == nil || !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%+v, %s: got %v, want a refusal saying %q", *c.policy, c.ip, err, c.reason)
		}
	}
	// The same addresses without a zone are judged as before.
	if err := (&Policy{AllowLoopback: true}).CheckIP(netip.MustParseAddr("::1")); err != nil {
		t.Errorf("loopback policy refused ::1: %v", err)
	}
	if err := (&Policy{AllowPrivate: true}).CheckIP(netip.MustParseAddr("fd12::1")); err != nil {
		t.Errorf("private policy refused fd12::1: %v", err)
	}
	if err := (&Policy{}).CheckIP(netip.MustParseAddr("2606:4700:4700::1111")); err != nil {
		t.Errorf("strict policy refused a global address: %v", err)
	}
}

// urlHost writes a zoned address as a URL host: in brackets, the % escaped.
func urlHost(s string) string {
	return "[" + strings.Replace(s, "%", "%25", 1) + "]"
}

func TestValidateURLRefusesZonedAddresses(t *testing.T) {
	for _, s := range zoned {
		for _, raw := range []string{"http://" + urlHost(s) + "/hook", "https://" + urlHost(s) + ":8443/hook"} {
			for name, p := range zonePolicies() {
				err := p.ValidateURL(raw)
				if err == nil {
					t.Errorf("%s policy accepted %s", name, raw)
					continue
				}
				if !errors.Is(err, ErrBlocked) {
					t.Errorf("%s policy, %s: error does not wrap ErrBlocked: %v", name, raw, err)
				}
			}
		}
	}
	// What the allow lists let through is unchanged.
	dev := zonePolicies()["exempt"]
	for _, raw := range []string{"http://localhost:9000/x", "http://[::1]:9000/x", "http://10.0.0.5/x", "http://[fd12::1]/x", "http://runner.internal:8095/fn"} {
		if err := dev.ValidateURL(raw); err != nil {
			t.Errorf("dev policy rejected %s: %v", raw, err)
		}
	}
}

func TestControlRefusesZonedAddresses(t *testing.T) {
	for _, s := range zoned {
		addr := net.JoinHostPort(s, "8080")
		for name, p := range zonePolicies() {
			err := p.Control("tcp6", addr, nil)
			if err == nil {
				t.Errorf("%s policy let the dial to %s through", name, addr)
				continue
			}
			if !errors.Is(err, ErrBlocked) {
				t.Errorf("%s policy, %s: error does not wrap ErrBlocked: %v", name, addr, err)
			}
		}
	}
	// An empty zone is not an address at all.
	if err := (&Policy{AllowLoopback: true, AllowPrivate: true}).Control("tcp6", "[fe80::1%]:8080", nil); !errors.Is(err, ErrBlocked) {
		t.Errorf("empty zone: got %v, want a refusal", err)
	}
	// The same addresses without a zone are judged as before.
	if err := (&Policy{AllowLoopback: true}).Control("tcp6", "[::1]:8080", nil); err != nil {
		t.Errorf("loopback policy refused [::1]:8080: %v", err)
	}
}

// loopbackInterface is the name of this machine's loopback interface (lo0 on
// macOS, lo on Linux), or "".
func loopbackInterface() string {
	ifs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 && i.Flags&net.FlagUp != 0 {
			return i.Name
		}
	}
	return ""
}

// A dial to the unspecified address with the loopback interface as its zone
// reaches a listener on ::1. The guard must refuse the connection itself,
// under the strict policy and under one that allows loopback and private
// destinations.
func TestDialContextRefusesZonedUnspecifiedAddress(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback here: %v", err)
	}
	srv := &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), ReadHeaderTimeout: time.Second},
	}
	srv.Start()
	defer srv.Close()
	lo := loopbackInterface()
	if lo == "" {
		t.Skip("no loopback interface found")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	target := net.JoinHostPort("::%"+lo, strconv.Itoa(port))

	// Without the guard, the dial reaches the listener (on the systems this
	// was seen on); logged, not asserted, since it is the kernel's behaviour.
	if c, err := net.DialTimeout("tcp", target, 2*time.Second); err == nil {
		t.Logf("an unguarded dial to %s reached the listener on [::1]:%d", target, port)
		_ = c.Close()
	} else {
		t.Logf("an unguarded dial to %s did not connect here: %v", target, err)
	}

	base := &net.Dialer{Timeout: 2 * time.Second}
	for name, p := range map[string]*Policy{"strict": {}, "dev": {AllowLoopback: true, AllowPrivate: true}} {
		client := &http.Client{Transport: &http.Transport{DialContext: p.DialContext(base)}, Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + urlHost("::%"+lo) + ":" + strconv.Itoa(port) + "/")
		if err == nil {
			_ = resp.Body.Close()
			t.Errorf("%s policy: the guarded client reached the listener through %s", name, target)
			continue
		}
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%s policy: the dial failed, but not by the guard: %v", name, err)
		}
	}
	// The listener itself is reachable where loopback is allowed.
	client := &http.Client{Transport: &http.Transport{DialContext: (&Policy{AllowLoopback: true}).DialContext(base)}, Timeout: 3 * time.Second}
	resp, err := client.Get("http://[::1]:" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatalf("loopback policy could not reach [::1]: %v", err)
	}
	_ = resp.Body.Close()
}
