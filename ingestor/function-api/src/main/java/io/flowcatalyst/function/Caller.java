package io.flowcatalyst.function;

import java.util.List;
import java.util.Set;

/// Who a [Request] came from. A sealed type, not a nullable field, so a
/// function author's `switch` enumerates the three cases the host can ever
/// hand it, one per `auth` mode an endpoint declares:
///
/// | `auth` | `Caller` |
/// |---|---|
/// | `webhook` | [Platform]: a verified delivery (subscription, direct dispatch job or scheduled job) |
/// | `platform` | [Principal]: a platform bearer token, verified by the host |
/// | `none` | [Anonymous]: the host checked nothing; the function authenticates itself, if at all |
///
/// These are the platform's caller kinds `webhook`, `principal` and
/// `anonymous` (`abi.Caller`).
public sealed interface Caller permits Caller.Platform, Caller.Principal, Caller.Anonymous {

    /// A verified webhook delivery. The one instance is shared: there is
    /// nothing to distinguish two platform deliveries by at this level; a
    /// function that needs to know *which* subscription or job fired reads
    /// [Webhook#event] / [Webhook#schedule].
    record Platform() implements Caller {
        public static final Platform INSTANCE = new Platform();
    }

    /// An authenticated platform principal: a user or service-account bearer
    /// token the host verified before this call reached [Function#handle].
    /// Email and name are deliberately not carried (a function has no
    /// business with them). The methods restate the platform SDK's helpers
    /// (`clients/fn-ts` `Principal`), so a function's own check answers as
    /// the platform's would.
    ///
    /// A `null` string becomes `""` and a `null` collection becomes empty, as
    /// in the other SDKs.
    ///
    /// @param id              the principal's id (JWT `sub`)
    /// @param type            the principal's kind (e.g. `USER`, `SERVICE`)
    /// @param tier            the tenancy tier (`ANCHOR` / `PARTNER` / `CLIENT`), `""` when absent
    /// @param clients         client ids this principal can access, verbatim (`*` for anchors)
    /// @param clientCodes     the client identifiers (e.g. `bevco`) matching [#clients]; `*` for anchors
    /// @param roles           assigned role codes
    /// @param applications    application entries: a bare id or an `id:code` pair; ignored when [#allApplications]
    /// @param allApplications access to every application, present and future
    /// @param permissions     permission codes held (4-segment, `*` wildcards allowed)
    record Principal(String id, String type, String tier, List<String> clients, List<String> clientCodes,
                     List<String> roles, List<String> applications, boolean allApplications,
                     Set<String> permissions) implements Caller {
        public Principal {
            id = id == null ? "" : id;
            type = type == null ? "" : type;
            tier = tier == null ? "" : tier;
            clients = Copies.list(clients);
            clientCodes = Copies.list(clientCodes);
            roles = Copies.list(roles);
            applications = Copies.list(applications);
            permissions = Copies.set(permissions);
        }

        /// True when the principal has anchor (platform-wide) scope.
        public boolean isAnchor() {
            return "ANCHOR".equals(tier);
        }

        /// True when the principal carries exactly this role code (no wildcarding).
        public boolean hasRole(String code) {
            return roles.contains(code);
        }

        /// True for an anchor, or when `clientId` is in [#clients].
        public boolean canAccessClient(String clientId) {
            return isAnchor() || clients.contains(clientId);
        }

        /// True for an anchor, or when `code` (the client identifier, e.g.
        /// `bevco`) is in [#clientCodes]. Exact match only.
        public boolean canAccessClientCode(String code) {
            return isAnchor() || clientCodes.contains(code);
        }

        /// True when the principal holds all-applications access, or when the
        /// application **id** is among [#applications].
        ///
        /// An entry matches when it equals `applicationId` or is an `id:code`
        /// pair whose id (the part before the first `:`) equals it, since the
        /// token's `applications` claim carries `id:code` pairs or the single
        /// `*` sentinel (meaning every application). Bare ids still match, as
        /// in fn-ts.
        public boolean canAccessApplication(String applicationId) {
            if (allApplications) return true;
            for (String entry : applications) {
                if (entry.equals("*") || entry.equals(applicationId)) return true;
                int colon = entry.indexOf(':');
                if (colon > 0 && entry.substring(0, colon).equals(applicationId)) return true;
            }
            return false;
        }

        /// True when the principal holds all-applications access, or when the
        /// application **code** (e.g. `bevco-integration`) is among
        /// [#applications]: an entry is `*`, the code itself, or an `id:code`
        /// pair whose code (the part after the first `:`) equals it.
        public boolean canAccessApplicationCode(String code) {
            if (allApplications) return true;
            for (String entry : applications) {
                if (entry.equals("*") || entry.equals(code)) return true;
                int colon = entry.indexOf(':');
                if (colon >= 0 && entry.substring(colon + 1).equals(code)) return true;
            }
            return false;
        }

        /// True when a held permission satisfies `required`: exact match, or a
        /// held code with the same number of `:`-separated segments whose
        /// non-wildcard (`*`) segments equal `required`'s. `null` is never
        /// satisfied.
        public boolean hasPermission(String required) {
            if (required == null) return false;
            for (String held : permissions) {
                if (matches(held, required)) return true;
            }
            return false;
        }

        /// Any of `required` is held.
        public boolean hasAnyPermission(String... required) {
            for (String r : required) {
                if (hasPermission(r)) return true;
            }
            return false;
        }

        /// Every one of `required` is held.
        public boolean hasAllPermissions(String... required) {
            for (String r : required) {
                if (!hasPermission(r)) return false;
            }
            return true;
        }

        private static boolean matches(String held, String required) {
            if (held.equals(required)) return true;
            var h = held.split(":", -1);
            var r = required.split(":", -1);
            if (h.length != r.length) return false;
            for (var i = 0; i < h.length; i++) {
                if (!h[i].equals("*") && !h[i].equals(r[i])) return false;
            }
            return true;
        }
    }

    /// The host performed no authentication (`auth: "none"`): inbound
    /// third-party webhooks and a function's own sessions land here. The one
    /// instance is shared, as for [Platform].
    record Anonymous() implements Caller {
        public static final Anonymous INSTANCE = new Anonymous();
    }
}
