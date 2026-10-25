package ingestion

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// The outbound guard for source addresses (owner ruling R4b): the hosts a
// definition makes the runtime connect to (an api-poll URL and its login or
// token URL, an sftp host, a RabbitMQ URI, a database's JDBC URL, a URL in an
// extension kind's settings). It is checked when a definition is saved and
// when one is tested; the runtime checks the address it actually connects to
// as well, after DNS, which this check cannot see.
//
// Unlike a delivery target, a source may live on a private network: an
// on-premises database or SFTP server is the normal case, so RFC 1918 and
// unique-local addresses are allowed. Never allowed, whatever the platform's
// delivery settings: loopback (the runtime's own machine), link-local
// (169.254.0.0/16 and fe80::/10, which hold the cloud metadata services),
// unspecified and multicast addresses, the cloud metadata addresses and names,
// and a host written as a number in a form only some resolvers read as an
// address (2130706433, 0x7f.1, 127.1). Host names are not resolved here.
var sourcePolicy = &netguard.Policy{AllowPrivate: true}

// metadataHosts are the names cloud providers give their instance metadata
// services.
var metadataHosts = []string{
	"metadata", "metadata.google.internal", "metadata.goog",
	"instance-data", "instance-data.ec2.internal",
	"metadata.azure.internal", "metadata.tencentyun.com", "metadata.platformequinix.com",
}

// metadataAddrs are metadata addresses outside the link-local range, which
// the private-network allowance would otherwise let through.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("100.100.100.200"), // Alibaba Cloud
	netip.MustParseAddr("fd00:ec2::254"),   // AWS, IPv6
}

var numericLabel = regexp.MustCompile(`^(0x[0-9a-f]*|[0-9]+)$`)

// CheckSourceHost reports why the runtime may not be sent to host, or nil.
// host is a bare name or address (an IPv6 one with or without brackets).
func CheckSourceHost(host string) error {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	h = strings.TrimSuffix(h, ".")
	switch {
	case h == "":
		return errors.New("must name a host")
	case h == "localhost" || strings.HasSuffix(h, ".localhost"):
		return fmt.Errorf("%w: %s is the runtime's own machine", netguard.ErrBlocked, h)
	case slices.Contains(metadataHosts, h):
		return fmt.Errorf("%w: %s is a cloud metadata service", netguard.ErrBlocked, h)
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		if slices.Contains(metadataAddrs, ip.Unmap().WithZone("")) {
			return fmt.Errorf("%w: %s is a cloud metadata address", netguard.ErrBlocked, ip)
		}
		return sourcePolicy.CheckIP(ip)
	}
	labels := strings.Split(h, ".")
	if !slices.ContainsFunc(labels, func(l string) bool { return !numericLabel.MatchString(l) }) {
		return fmt.Errorf("%w: %s is a number, not a host name or a plain IP address", netguard.ErrBlocked, h)
	}
	return nil
}

// sourceAddress is one outbound address of a source: the field it is in, the
// value as written, and the hosts it names.
type sourceAddress struct {
	field, value string
	hosts        []string
	// unreadable is set when no host could be read from the value.
	unreadable string
}

// sourceAddresses lists the outbound addresses of src. An http source is
// pushed to and s3 and sqs name AWS regions, so they have none.
func sourceAddresses(src Source) []sourceAddress {
	var out []sourceAddress
	addURL := func(field, raw string) {
		u := parseURL(raw)
		if u == nil || u.Hostname() == "" {
			return // the shape check already says what is wrong
		}
		out = append(out, sourceAddress{field: field, value: raw, hosts: []string{u.Hostname()}})
	}
	switch s := src.(type) {
	case Sftp:
		out = append(out, sourceAddress{field: "source.host", value: s.Host, hosts: []string{s.Host}})
	case RabbitMQ:
		addURL("source.uri", s.URI)
	case DatabasePoll:
		a := sourceAddress{field: "source.jdbcUrl", value: s.JDBCURL}
		a.hosts, a.unreadable = jdbcHosts(s.JDBCURL)
		out = append(out, a)
	case APIPoll:
		addURL("source.url", s.URL)
		switch auth := s.Auth.(type) {
		case ClientCredentials:
			addURL("source.auth.tokenUrl", auth.TokenURL)
		case Login:
			addURL("source.auth.url", auth.URL)
		}
	case Custom:
		// The platform does not know an extension's settings; a value that is
		// an absolute URL is treated as an address.
		for _, name := range sortedKeys(s.Settings) {
			for _, text := range settingTexts(s.Settings[name]) {
				if u, err := url.Parse(text); err == nil && u.Scheme != "" && u.Host != "" {
					out = append(out, sourceAddress{field: "source.settings." + name, value: text, hosts: []string{u.Hostname()}})
				}
			}
		}
	}
	return out
}

// addresses is the save-time check of every outbound address of src. An
// empty host is the shape check's to report.
func (v *checker) addresses(src Source) {
	for _, a := range sourceAddresses(src) {
		if a.unreadable != "" {
			v.add(a.field, "INVALID", a.unreadable)
			continue
		}
		for _, h := range a.hosts {
			if strings.TrimSpace(h) == "" {
				continue
			}
			if err := CheckSourceHost(h); err != nil {
				v.add(a.field, "INVALID", "is not allowed: "+strings.TrimPrefix(err.Error(), netguard.ErrBlocked.Error()+": "))
				break
			}
		}
	}
	if p, ok := src.(DatabasePoll); ok {
		if prop := unsafeJDBCProperty(p.JDBCURL); prop != "" {
			v.add("source.jdbcUrl", "INVALID", "may not set "+prop+": it makes the driver load code or read or write files on the runtime")
		}
	}
}

// CheckSourceAddresses is the outbound guard on its own, for a definition
// saved before it existed (a saved ingestion being tested).
func CheckSourceAddresses(src Source) Problems {
	var v checker
	v.addresses(src)
	return v.problems
}

var (
	jdbcHostProperty = regexp.MustCompile(`(?i)(?:^|[;?&(])\s*(?:host|servername|server)\s*=\s*([^;&)\s]+)`)
	oracleHost       = regexp.MustCompile(`(?i)\bhost\s*=\s*([^)\s]+)`)
)

// jdbcHosts reads the database hosts of a JDBC URL: the authority after //
// (one host or several, comma-separated, with or without ports, IPv6 in
// brackets, a SQL Server \instance), Oracle's @host:port and
// (DESCRIPTION=(ADDRESS=(HOST=...))) forms, MySQL's address=(host=...) form,
// and host or serverName properties. A URL with no host to read (an embedded
// database, a file, an Oracle TNS alias) is refused: the runtime would open
// something on its own machine, or something this check cannot see.
func jdbcHosts(raw string) ([]string, string) {
	const unreadable = "must name the database server, as jdbc:<driver>://host:port/... (or Oracle's jdbc:oracle:thin:@host:port...)"
	rest, ok := strings.CutPrefix(raw, "jdbc:")
	if !ok {
		return nil, "" // the shape check refuses it
	}
	var hosts []string
	if strings.HasPrefix(strings.ToLower(rest), "oracle:") {
		_, after, found := strings.Cut(rest, "@")
		if !found {
			return nil, unreadable
		}
		switch {
		case strings.Contains(after, "("):
			for _, m := range oracleHost.FindAllStringSubmatch(after, -1) {
				hosts = append(hosts, m[1])
			}
		case !strings.ContainsAny(after, ":/"):
			// A TNS alias: the host is in a tnsnames.ora on the runtime.
			return nil, unreadable
		default:
			if h := hostOf(strings.TrimPrefix(after, "//")); h != "" {
				hosts = append(hosts, h)
			}
		}
	} else {
		_, after, found := strings.Cut(rest, "//")
		if !found {
			return nil, unreadable
		}
		authority := after
		if i := strings.IndexAny(authority, "/?;"); i >= 0 {
			authority = authority[:i]
		}
		if !strings.HasPrefix(strings.ToLower(authority), "address=") {
			for part := range strings.SplitSeq(authority, ",") {
				if _, hostPort, hasUser := strings.Cut(part, "@"); hasUser {
					part = hostPort
				}
				if h, _, _ := strings.Cut(part, `\`); h != "" {
					part = h
				}
				if h := hostOf(part); h != "" {
					hosts = append(hosts, h)
				}
			}
		}
		for _, m := range jdbcHostProperty.FindAllStringSubmatch(after, -1) {
			hosts = append(hosts, m[1])
		}
	}
	if len(hosts) == 0 {
		return nil, unreadable
	}
	return hosts, ""
}

// hostOf is the host of host[:port][...], an IPv6 address in brackets kept
// whole.
func hostOf(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end > 0 {
			return s[1:end]
		}
		return ""
	}
	if i := strings.IndexAny(s, ":/;"); i >= 0 {
		s = s[:i]
	}
	return s
}

// unsafeJDBCProperties are driver properties that make a JDBC driver
// instantiate a class named in the URL, deserialize what the server sends, or
// read or write files on the runtime's machine. A polling source needs none
// of them.
var unsafeJDBCProperties = []string{
	"socketfactory", "socketfactoryarg", "sslfactory", "sslfactoryarg", "sslhostnameverifier",
	"sslpasswordcallback", "authenticationpluginclassname", "loggerfile", "allowloadlocalinfile",
	"allowloadlocalinfileinpath", "allowurlinlocalinfile", "autodeserialize", "queryinterceptors",
	"statementinterceptors", "exceptioninterceptors", "connectionlifecycleinterceptors", "init",
}

var jdbcProperty = regexp.MustCompile(`[;?&(]\s*([A-Za-z][A-Za-z0-9_.]*)\s*=`)

// unsafeJDBCProperty names the first unsafe property the URL sets, "" for none.
func unsafeJDBCProperty(raw string) string {
	for _, m := range jdbcProperty.FindAllStringSubmatch(raw, -1) {
		if slices.Contains(unsafeJDBCProperties, strings.ToLower(m[1])) {
			return m[1]
		}
	}
	return ""
}

// settingTexts is the text in a setting's value: the value itself, or the
// items of a list.
func settingTexts(value any) []string {
	switch x := value.(type) {
	case string:
		return []string{x}
	case []string:
		return x
	case []any:
		var out []string
		for _, item := range x {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
