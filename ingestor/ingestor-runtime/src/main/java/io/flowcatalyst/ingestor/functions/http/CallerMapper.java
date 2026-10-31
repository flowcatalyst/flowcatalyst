package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.framework.auth.Principal;
import io.flowcatalyst.function.Caller;
import java.util.ArrayList;
import java.util.List;
import org.jspecify.annotations.Nullable;

/// Maps a verified platform access token's principal to the function API's [Caller.Principal], exactly as the
/// Go runner builds `abi.Caller` from the token's claims (`internal/functions/runner/auth.go`):
///
/// - `id` is `sub`, `type` the `type` claim (`USER` or `SERVICE`), `tier` the `tier` claim (`""` when absent);
/// - `clients` are the ids of the `clients` entries (`id:identifier` entries reduced to `id`, `*` stays);
///   `clientCodes` their identifiers (`*` stays; an entry with no colon or an empty identifier gives none);
/// - `roles`, `applications` as the token has them; `permissions` is the token's `scope`;
/// - `allApplications` is true when `applications` holds the `*` sentinel.
///
/// Divergences from the Go runner, both from the framework's [Principal]: it drops the `all_applications` claim
/// (the sentinel in `applications` stands for it), and reads any `type` claim other than `SERVICE` as `USER`.
public final class CallerMapper {

    private CallerMapper() {
    }

    /// The platform permission an explicit `@v{n}` call needs.
    public static final String PERM_VERSION_INVOKE = "platform:function:version:invoke";

    public static Caller.Principal principal(Principal p) {
        List<String> clients = p.clients();
        return new Caller.Principal(
                p.id().value(),
                p.kind().name(),
                p.tier() == null ? "" : p.tier(),
                clientIds(clients),
                clientCodes(clients),
                p.roles(),
                p.applications(),
                p.applications().contains("*"),
                p.scopes());
    }

    /// `clientIDs`: the token's "id:identifier" client entries reduced to ids ("*" stays).
    static List<String> clientIds(List<String> in) {
        List<String> out = new ArrayList<>(in.size());
        for (String c : in) {
            int colon = c.indexOf(':');
            out.add(colon < 0 ? c : c.substring(0, colon));
        }
        return out;
    }

    /// `clientCodes`: the identifier half of each "id:identifier" client entry.
    static List<String> clientCodes(List<String> in) {
        List<String> out = new ArrayList<>();
        for (String c : in) {
            if (c.equals("*")) {
                out.add(c);
                continue;
            }
            int colon = c.indexOf(':');
            if (colon >= 0 && colon < c.length() - 1) {
                out.add(c.substring(colon + 1));
            }
        }
        return out;
    }

    /// The platform's reach rule for a function (`canReach`): anchor callers reach everything;
    /// platform-owned functions (no client) are anchor-only; otherwise the caller must hold the function's client.
    public static boolean canReach(Caller.Principal caller, @Nullable String clientId) {
        if (caller.isAnchor()) {
            return true;
        }
        if (clientId == null) {
            return false;
        }
        for (String id : caller.clients()) {
            if (id.equals("*") || id.equals(clientId)) {
                return true;
            }
        }
        return false;
    }
}
