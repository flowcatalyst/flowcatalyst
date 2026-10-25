package io.flowcatalyst.ingestor.runtime;

import java.net.Inet4Address;
import java.net.Inet6Address;
import java.net.InetAddress;
import java.net.URI;
import java.net.UnknownHostException;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;

/**
 * Keeps a source from reaching what only the runtime's own host should: an address a definition
 * names (an SFTP or RabbitMQ host, a database, an API and its token endpoint) is refused when it
 * resolves to a loopback, link-local ({@code 169.254.0.0/16}, {@code fe80::/10}), unspecified or
 * "this network" address, or a cloud metadata address. Private ranges are allowed: a source in the
 * same network is the usual case.
 *
 * <p>The name is resolved, and every address it resolves to checked, when the route starts and
 * again before each connection the runtime makes for the source. The JVM keeps a resolved name for
 * {@code networkaddress.cache.ttl} (30 seconds), and the client that connects right after reads the
 * same answer, so a name that resolves elsewhere a moment later (DNS rebinding) does not get past.
 * A name that does not resolve is not refused here: the connection fails on its own.
 *
 * <p>Loopback is allowed only where the runtime runs beside its sources on purpose: {@code APP_ENV}
 * {@code local} or {@code test} (fcdev). S3 and SQS are not checked: their addresses are AWS's, or the
 * runtime's own {@code INGEST_AWS_ENDPOINT}.
 */
final class OutboundGuard {

  /** Cloud metadata addresses outside the link-local range. */
  private static final Set<String> METADATA = Set.of("100.100.100.200", "fd00:ec2:0:0:0:0:0:254");

  /** {@code jdbc:sub[:sub]://host[:port][,host[:port]]...}: the hosts, up to the path, the query or a ';'. */
  private static final Pattern JDBC_AUTHORITY = Pattern.compile("^jdbc:[A-Za-z0-9]+(?::[A-Za-z0-9]+)*://([^/?;]*)");

  /** Oracle's {@code jdbc:oracle:thin:@host:port...} and {@code @//host:port/service}. */
  private static final Pattern JDBC_AT = Pattern.compile("^jdbc:oracle:[a-z]+:@(?://)?(\\[[^\\]]+\\]|[^:/?]+)");

  private final boolean loopback;

  private OutboundGuard(boolean loopback) {
    this.loopback = loopback;
  }

  /**
   * The guard for the environment the runtime runs in.
   *
   * @param environment {@code APP_ENV}
   * @return a guard that allows loopback in {@code local} and {@code test} only
   */
  static OutboundGuard forEnvironment(String environment) {
    return new OutboundGuard("local".equals(environment) || "test".equals(environment));
  }

  /**
   * A guard that allows loopback, for tests whose sources run on this machine.
   *
   * @return the guard
   */
  static OutboundGuard allowingLoopback() {
    return new OutboundGuard(true);
  }

  /**
   * A guard that refuses loopback too.
   *
   * @return the guard
   */
  static OutboundGuard strict() {
    return new OutboundGuard(false);
  }

  /**
   * Refuses a host that resolves to an address no source may reach.
   *
   * @param host a name or an address literal
   * @param what what it is, for the message, such as {@code the sftp host}
   * @throws UnsupportedIngestionException when it is refused; the message names the host and why
   */
  void check(String host, String what) {
    String name = host.startsWith("[") && host.endsWith("]") ? host.substring(1, host.length() - 1) : host;
    InetAddress[] addresses;
    try {
      addresses = InetAddress.getAllByName(name);
    } catch (UnknownHostException | SecurityException e) {
      return;
    }
    for (InetAddress address : addresses) {
      String why = forbidden(address);
      if (why != null) {
        throw new UnsupportedIngestionException(
            what + " " + host + " resolves to " + address.getHostAddress() + ", " + why + ", which a source may not reach");
      }
    }
  }

  /**
   * Refuses every host of a JDBC URL in the {@code //host} form or Oracle's {@code @host} form; a
   * URL in another form names no host this can read and is not checked.
   *
   * @param jdbcUrl the URL
   */
  void checkJdbc(String jdbcUrl) {
    for (String host : jdbcHosts(jdbcUrl)) {
      check(host, "the database host");
    }
  }

  /**
   * Refuses the host of an http(s) address.
   *
   * @param url the address
   * @param what what it is, for the message
   */
  void checkUrl(String url, String what) {
    String host;
    try {
      host = new URI(url).getHost();
    } catch (java.net.URISyntaxException e) {
      return;
    }
    if (host != null) {
      check(host, what);
    }
  }

  /** The hosts of a JDBC URL, without ports. */
  static List<String> jdbcHosts(String jdbcUrl) {
    List<String> hosts = new ArrayList<>();
    Matcher at = JDBC_AT.matcher(jdbcUrl);
    if (at.find()) {
      hosts.add(at.group(1));
      return hosts;
    }
    Matcher authority = JDBC_AUTHORITY.matcher(jdbcUrl);
    if (!authority.find()) {
      return hosts;
    }
    String all = authority.group(1);
    int login = all.lastIndexOf('@');
    if (login >= 0) {
      all = all.substring(login + 1);
    }
    for (String part : all.split(",")) {
      String host = part.strip();
      if (host.startsWith("[")) {
        int end = host.indexOf(']');
        host = end > 0 ? host.substring(0, end + 1) : host;
      } else {
        int colon = host.indexOf(':');
        host = colon >= 0 ? host.substring(0, colon) : host;
      }
      if (!host.isEmpty()) {
        hosts.add(host);
      }
    }
    return hosts;
  }

  /** Why the address may not be reached, or null when it may. */
  @Nullable String forbidden(InetAddress address) {
    InetAddress plain = embeddedIpv4(address);
    if (plain != address) {
      String why = forbidden(plain);
      if (why != null) {
        return why;
      }
    }
    if (address.isAnyLocalAddress()) {
      return "an unspecified address";
    }
    if (address.isLoopbackAddress()) {
      return loopback ? null : "a loopback address";
    }
    if (address.isLinkLocalAddress()) {
      return "a link-local address (where cloud metadata services answer)";
    }
    if (address instanceof Inet4Address && (address.getAddress()[0] == 0)) {
      return "a 'this network' address";
    }
    if (METADATA.contains(address.getHostAddress().toLowerCase(Locale.ROOT))) {
      return "a cloud metadata address";
    }
    return null;
  }

  /** The IPv4 address an IPv6 one carries (IPv4-compatible, NAT64 {@code 64:ff9b::/96}), or the address itself. */
  private static InetAddress embeddedIpv4(InetAddress address) {
    if (!(address instanceof Inet6Address v6)) {
      return address;
    }
    byte[] b = v6.getAddress();
    boolean compatible = true;
    for (int i = 0; i < 12; i++) {
      compatible &= b[i] == 0;
    }
    // Not :: and ::1, which are IPv6's own unspecified and loopback addresses.
    compatible &= b[12] != 0;
    boolean nat64 = b[0] == 0x00 && b[1] == 0x64 && (b[2] & 0xff) == 0xff && (b[3] & 0xff) == 0x9b;
    for (int i = 4; i < 12 && nat64; i++) {
      nat64 = b[i] == 0;
    }
    if (!compatible && !nat64) {
      return address;
    }
    try {
      return InetAddress.getByAddress(new byte[] {b[12], b[13], b[14], b[15]});
    } catch (UnknownHostException e) {
      return address;
    }
  }
}
