package io.flowcatalyst.ingestor.functions.context;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Consumer;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// A PostgreSQL DSN in the forms the Go runner hands to `pgxpool.ParseConfig`, turned into what pgjdbc takes: a
/// JDBC URL (which never carries the credentials), the user and password, and driver properties.
///
/// Two forms are read: the URL form `postgres://` / `postgresql://user:pass@host:port,host2:port2/db?params`
/// (percent-encoded user and password; `+` in the query is a space, as `net/url` reads it) and the keyword/value
/// form `host=... port=... user=... password=... dbname=... sslmode=...` (values may be single quoted, with `\`
/// escapes). What cannot be represented is refused with a [DsnException] whose message names the problem and
/// **never carries any part of the DSN** (the DSN holds a password): a Unix-socket host, a missing host or user,
/// an unknown `sslmode`, a port that is not a number.
///
/// pgx-only parameters (pool sizing, statement cache and the like) are ignored; the key, never the value, is
/// reported to `onIgnoredKey`.
public final class PostgresDsn {

    private static final Logger LOG = LoggerFactory.getLogger(PostgresDsn.class);

    /// pgx's `sslmode` values; pgjdbc takes the same names.
    private static final Set<String> SSL_MODES = Set.of("disable", "allow", "prefer", "require", "verify-ca", "verify-full");

    /// `target_session_attrs` (pgx/libpq) to pgjdbc's `targetServerType`.
    private static final Map<String, String> SESSION_ATTRS = Map.of(
            "any", "any",
            "read-write", "primary",
            "read-only", "secondary",
            "primary", "primary",
            "standby", "secondary",
            "prefer-standby", "preferSecondary");

    /// One pgjdbc connection target. The password is not in [#jdbcUrl] and not in [#toString].
    public record Target(String jdbcUrl, String user, @Nullable String password, Map<String, String> properties) {
        public Target {
            properties = Map.copyOf(properties);
        }

        @Override
        public String toString() {
            return "Target[" + jdbcUrl + ", user=" + user + ", password=" + (password == null ? "none" : "***") + "]";
        }
    }

    /// The DSN cannot be used. The message is safe to log and to show.
    public static final class DsnException extends IllegalArgumentException {
        private static final long serialVersionUID = 1L;

        DsnException(String message) {
            super(message);
        }
    }

    private PostgresDsn() {
    }

    public static Target parse(String dsn) {
        return parse(dsn, key -> LOG.debug("ignoring the DSN parameter '{}': pgjdbc has no equivalent", key));
    }

    /// @param onIgnoredKey told the name (never the value) of each parameter that is dropped
    /// @throws DsnException the DSN is not valid or cannot be represented
    public static Target parse(String dsn, Consumer<String> onIgnoredKey) {
        String s = dsn.strip();
        if (s.isEmpty()) {
            throw new DsnException("the DSN is empty");
        }
        Raw raw = s.startsWith("postgres://") || s.startsWith("postgresql://") ? parseUrl(s) : parseKeywords(s);
        return build(raw, onIgnoredKey);
    }

    /// The pieces both forms reduce to, before they are mapped.
    private record Raw(List<String> hosts, List<String> ports, @Nullable String user, @Nullable String password,
            @Nullable String database, Map<String, String> params) {
    }

    // ---- URL form ----

    private static Raw parseUrl(String s) {
        String rest = s.substring(s.indexOf("://") + 3);
        int frag = rest.indexOf('#');
        if (frag >= 0) {
            rest = rest.substring(0, frag);
        }
        String query = "";
        int q = rest.indexOf('?');
        if (q >= 0) {
            query = rest.substring(q + 1);
            rest = rest.substring(0, q);
        }
        String path = "";
        int slash = rest.indexOf('/');
        if (slash >= 0) {
            path = rest.substring(slash + 1);
            rest = rest.substring(0, slash);
        }
        String user = null;
        String password = null;
        int at = rest.lastIndexOf('@');
        if (at >= 0) {
            String info = rest.substring(0, at);
            rest = rest.substring(at + 1);
            int colon = info.indexOf(':');
            user = percentDecode(colon >= 0 ? info.substring(0, colon) : info, false, "user");
            if (colon >= 0) {
                password = percentDecode(info.substring(colon + 1), false, "password");
            }
        }
        List<String> hosts = new ArrayList<>();
        List<String> ports = new ArrayList<>();
        if (!rest.isEmpty()) {
            for (String hp : rest.split(",", -1)) {
                String host = hp;
                String port = "";
                if (hp.startsWith("[")) {
                    int end = hp.indexOf(']');
                    if (end < 0) {
                        throw new DsnException("a host in the DSN has an unclosed '['");
                    }
                    host = hp.substring(1, end);
                    String after = hp.substring(end + 1);
                    if (!after.isEmpty()) {
                        if (!after.startsWith(":")) {
                            throw new DsnException("a host in the DSN is not followed by a port");
                        }
                        port = after.substring(1);
                    }
                } else {
                    int colon = hp.lastIndexOf(':');
                    if (colon >= 0) {
                        host = hp.substring(0, colon);
                        port = hp.substring(colon + 1);
                    }
                }
                hosts.add(percentDecode(host, false, "host"));
                ports.add(port);
            }
        }
        String database = path.isEmpty() ? null : percentDecode(path, false, "database name");
        Map<String, String> params = new LinkedHashMap<>();
        if (!query.isEmpty()) {
            for (String pair : query.split("&")) {
                if (pair.isEmpty()) {
                    continue;
                }
                int eq = pair.indexOf('=');
                String key = percentDecode(eq >= 0 ? pair.substring(0, eq) : pair, true, "parameter");
                String value = eq >= 0 ? percentDecode(pair.substring(eq + 1), true, "parameter") : "";
                params.put(key, value);
            }
        }
        return new Raw(hosts, ports, user, password, database, params);
    }

    private static String percentDecode(String in, boolean plusIsSpace, String what) {
        byte[] out = new byte[in.length() * 3];
        int n = 0;
        byte[] src = in.getBytes(StandardCharsets.UTF_8);
        for (int i = 0; i < src.length; i++) {
            byte b = src[i];
            if (b == '%') {
                if (i + 2 >= src.length) {
                    throw new DsnException("the DSN's " + what + " has a bad percent escape");
                }
                int hi = Character.digit(src[i + 1], 16);
                int lo = Character.digit(src[i + 2], 16);
                if (hi < 0 || lo < 0) {
                    throw new DsnException("the DSN's " + what + " has a bad percent escape");
                }
                out[n++] = (byte) (hi * 16 + lo);
                i += 2;
            } else if (b == '+' && plusIsSpace) {
                out[n++] = ' ';
            } else {
                out[n++] = b;
            }
        }
        return new String(out, 0, n, StandardCharsets.UTF_8);
    }

    // ---- keyword/value form ----

    private static Raw parseKeywords(String s) {
        Map<String, String> kv = new LinkedHashMap<>();
        int i = 0;
        int len = s.length();
        while (i < len) {
            while (i < len && Character.isWhitespace(s.charAt(i))) {
                i++;
            }
            if (i >= len) {
                break;
            }
            int keyStart = i;
            while (i < len && s.charAt(i) != '=' && !Character.isWhitespace(s.charAt(i))) {
                i++;
            }
            String key = s.substring(keyStart, i);
            while (i < len && Character.isWhitespace(s.charAt(i))) {
                i++;
            }
            if (i >= len || s.charAt(i) != '=' || key.isEmpty()) {
                throw new DsnException("the DSN is neither a postgres:// URL nor key=value pairs");
            }
            i++;
            while (i < len && Character.isWhitespace(s.charAt(i))) {
                i++;
            }
            StringBuilder value = new StringBuilder();
            if (i < len && s.charAt(i) == '\'') {
                i++;
                boolean closed = false;
                while (i < len) {
                    char c = s.charAt(i++);
                    if (c == '\\' && i < len) {
                        value.append(s.charAt(i++));
                    } else if (c == '\'') {
                        closed = true;
                        break;
                    } else {
                        value.append(c);
                    }
                }
                if (!closed) {
                    throw new DsnException("the DSN has an unterminated quoted value");
                }
            } else {
                while (i < len && !Character.isWhitespace(s.charAt(i))) {
                    char c = s.charAt(i++);
                    if (c == '\\' && i < len) {
                        value.append(s.charAt(i++));
                    } else {
                        value.append(c);
                    }
                }
            }
            kv.put(key, value.toString());
        }
        String hostList = kv.remove("host");
        String portList = kv.remove("port");
        List<String> hosts = new ArrayList<>();
        if (hostList != null && !hostList.isEmpty()) {
            hosts.addAll(List.of(hostList.split(",", -1)));
        }
        List<String> ports = new ArrayList<>();
        if (portList != null && !portList.isEmpty()) {
            ports.addAll(List.of(portList.split(",", -1)));
        }
        if (ports.size() > 1 && ports.size() != hosts.size()) {
            throw new DsnException("the DSN has " + ports.size() + " ports for " + hosts.size() + " hosts");
        }
        List<String> perHost = new ArrayList<>();
        for (int h = 0; h < hosts.size(); h++) {
            perHost.add(ports.isEmpty() ? "" : ports.size() == 1 ? ports.get(0) : ports.get(h));
        }
        return new Raw(hosts, perHost, kv.remove("user"), kv.remove("password"), kv.remove("dbname"), kv);
    }

    // ---- mapping ----

    private static Target build(Raw raw, Consumer<String> onIgnoredKey) {
        Map<String, String> params = new LinkedHashMap<>(raw.params());
        // pgx also reads host, port, user, password and dbname given as URL query parameters.
        List<String> hosts = new ArrayList<>(raw.hosts());
        List<String> ports = new ArrayList<>(raw.ports());
        String qHost = params.remove("host");
        if (hosts.isEmpty() && qHost != null && !qHost.isEmpty()) {
            hosts.addAll(List.of(qHost.split(",", -1)));
            ports.clear();
            for (int i = 0; i < hosts.size(); i++) {
                ports.add("");
            }
        }
        String qPort = params.remove("port");
        if (qPort != null && !qPort.isEmpty()) {
            String[] fromQuery = qPort.split(",", -1);
            for (int i = 0; i < ports.size(); i++) {
                if (ports.get(i).isEmpty()) {
                    ports.set(i, fromQuery[Math.min(i, fromQuery.length - 1)]);
                }
            }
        }
        if (hosts.isEmpty()) {
            throw new DsnException("the DSN names no host (a Unix-socket or environment default cannot be represented)");
        }
        StringBuilder hostPart = new StringBuilder();
        for (int i = 0; i < hosts.size(); i++) {
            String host = hosts.get(i);
            if (host.isEmpty()) {
                throw new DsnException("the DSN has an empty host");
            }
            if (host.startsWith("/") || host.startsWith("@")) {
                throw new DsnException("a Unix-socket host cannot be represented");
            }
            String portText = i < ports.size() ? ports.get(i) : "";
            int port = 5432;
            if (!portText.isEmpty()) {
                try {
                    port = Integer.parseInt(portText);
                } catch (NumberFormatException e) {
                    throw new DsnException("the DSN's port is not a number");
                }
                if (port < 1 || port > 65535) {
                    throw new DsnException("the DSN's port is out of range");
                }
            }
            if (i > 0) {
                hostPart.append(',');
            }
            hostPart.append(host.indexOf(':') >= 0 ? "[" + host + "]" : host).append(':').append(port);
        }

        String user = firstNonEmpty(raw.user(), params.remove("user"));
        if (user == null) {
            throw new DsnException("the DSN names no user");
        }
        String passParam = params.remove("password");
        String password = raw.password() != null ? raw.password() : passParam;
        if (password != null && password.isEmpty()) {
            password = null;
        }
        String database = firstNonEmpty(raw.database(), params.remove("dbname"));
        if (database == null) {
            database = user; // pgx's default
        }

        Map<String, String> props = new LinkedHashMap<>();
        for (Map.Entry<String, String> e : params.entrySet()) {
            String key = e.getKey();
            String value = e.getValue();
            switch (key) {
                case "sslmode" -> {
                    if (!SSL_MODES.contains(value)) {
                        throw new DsnException("the DSN's sslmode is not one of " + String.join(", ", new java.util.TreeSet<>(SSL_MODES)));
                    }
                    props.put("sslmode", value);
                }
                case "sslrootcert", "sslcert", "sslkey" -> props.put(key, value);
                case "application_name" -> props.put("ApplicationName", value);
                case "connect_timeout" -> {
                    int seconds;
                    try {
                        seconds = Integer.parseInt(value);
                    } catch (NumberFormatException ex) {
                        throw new DsnException("the DSN's connect_timeout is not a number of seconds");
                    }
                    if (seconds < 0) {
                        throw new DsnException("the DSN's connect_timeout is negative");
                    }
                    if (seconds > 0) {
                        props.put("connectTimeout", Integer.toString(seconds));
                        props.put("loginTimeout", Integer.toString(seconds));
                    }
                }
                case "search_path" -> props.put("currentSchema", value);
                case "options" -> props.put("options", value);
                case "target_session_attrs" -> {
                    String mapped = SESSION_ATTRS.get(value);
                    if (mapped == null) {
                        throw new DsnException("the DSN's target_session_attrs is not one pgjdbc can represent");
                    }
                    props.put("targetServerType", mapped);
                }
                default -> onIgnoredKey.accept(key);
            }
        }
        String url = "jdbc:postgresql://" + hostPart + "/" + URLEncoder.encode(database, StandardCharsets.UTF_8);
        return new Target(url, user, password, props);
    }

    private static @Nullable String firstNonEmpty(@Nullable String a, @Nullable String b) {
        if (a != null && !a.isEmpty()) {
            return a;
        }
        return b != null && !b.isEmpty() ? b : null;
    }
}
