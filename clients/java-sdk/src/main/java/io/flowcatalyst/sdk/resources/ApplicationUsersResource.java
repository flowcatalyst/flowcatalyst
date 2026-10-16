package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.ApplicationInvitationList;
import io.flowcatalyst.sdk.generated.model.ApplicationUser;
import io.flowcatalyst.sdk.generated.model.ApplicationUserList;
import io.flowcatalyst.sdk.generated.model.InviteApplicationUserRequest;
import io.flowcatalyst.sdk.generated.model.InviteApplicationUserResponse;
import io.flowcatalyst.sdk.generated.model.ResendInvitationRequest;
import io.flowcatalyst.sdk.generated.model.ResendInvitationResponse;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * An application's own users and invitations, for its admin pages: the calls an application
 * makes as its own service account about the users it owns.
 *
 * <p>An application's users are the CLIENT-scope users homed at a client the application is
 * enabled for who hold at least one of its roles. Every call is confined to the applications the
 * credential is bound to; another application's code is refused with {@code 403}, and a user the
 * application does not own answers {@code 404}. Responses never carry a password hash, a token or
 * an invite link, except the link a caller asked for at {@link #invite} or {@link #resend}.
 *
 * <p>Permissions (the built-in {@code platform:application-service} role): {@code ...:user:view},
 * {@code ...:user:create}, {@code ...:invitation:view}, {@code ...:invitation:manage}.
 */
public final class ApplicationUsersResource {

    private final Transport transport;

    public ApplicationUsersResource(Transport transport) {
        this.transport = transport;
    }

    /**
     * Optional filters for {@link #list}. {@code roles} are the application's roles (a role of
     * another application matches nothing); {@code page} is 0-based; {@code pageSize} defaults to
     * 50 and is at most 200; {@code sortField} is {@code name}, {@code email} or {@code createdAt}.
     */
    public record Filters(
            String q,
            Boolean active,
            String clientId,
            List<String> roles,
            Integer page,
            Integer pageSize,
            String sortField,
            String sortOrder) {
        public static Filters none() {
            return new Filters(null, null, null, null, null, null, null, null);
        }

        public static Filters page(int page, int pageSize) {
            return new Filters(null, null, null, null, page, pageSize, null, null);
        }
    }

    /** Lists the application's users. */
    public ApplicationUserList list(String applicationCode, Filters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        if (filters != null) {
            query.put("q", filters.q());
            query.put("active", filters.active());
            query.put("clientId", filters.clientId());
            query.put("roles", EventsResource.csv(filters.roles()));
            query.put("page", filters.page());
            query.put("pageSize", filters.pageSize());
            query.put("sortField", filters.sortField());
            query.put("sortOrder", filters.sortOrder());
        }
        return transport.get(base(applicationCode) + "/users", query, ApplicationUserList.class);
    }

    /** One of the application's users; {@code 404} when the id is not one of them. */
    public ApplicationUser get(String applicationCode, String userId) {
        return transport.get(
                base(applicationCode) + "/users/" + Transport.enc(userId), null, ApplicationUser.class);
    }

    /**
     * Invites (creates) a user at one of the application's enabled clients with at least one of
     * its roles. {@code sendInvitation} (default {@code true}) has the platform email the
     * invitation; {@code returnInviteLink} returns the live 72-hour link as {@code inviteLink}
     * instead (and suppresses the email), a bearer credential: treat it like a password and
     * never log it; {@code inviteRedirectUri} is where the invitee lands after setting a password.
     */
    public InviteApplicationUserResponse invite(String applicationCode, InviteApplicationUserRequest data) {
        return transport.post(base(applicationCode) + "/users", data, InviteApplicationUserResponse.class);
    }

    /**
     * Lists the outstanding invitations: users who have not set a password. {@code status} is
     * {@code PENDING} (a live link exists), {@code EXPIRED} (resend it) or {@code null} for both.
     */
    public ApplicationInvitationList invitations(
            String applicationCode, String status, Integer page, Integer pageSize) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("status", status);
        query.put("page", page);
        query.put("pageSize", pageSize);
        return transport.get(base(applicationCode) + "/invitations", query, ApplicationInvitationList.class);
    }

    /**
     * Resends a user's invitation: mints a fresh link (the earlier one stops working) and emails
     * it, or with {@code returnInviteLink} returns it. {@code body} may be {@code null} (email it).
     */
    public ResendInvitationResponse resend(String applicationCode, String userId, ResendInvitationRequest body) {
        return transport.post(
                base(applicationCode) + "/invitations/" + Transport.enc(userId) + "/resend",
                body == null ? new ResendInvitationRequest() : body,
                ResendInvitationResponse.class);
    }

    /**
     * Revokes an invitation: deletes the link and deactivates the (never signed-in) user. The user
     * record stays; reactivating it is a console action.
     */
    public void revoke(String applicationCode, String userId) {
        transport.delete(base(applicationCode) + "/invitations/" + Transport.enc(userId), Void.class);
    }

    private static String base(String applicationCode) {
        return "/api/applications/" + Transport.enc(applicationCode);
    }
}
