package io.flowcatalyst.sdk.resources;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.error.SdkError;
import io.flowcatalyst.sdk.generated.model.ApplicationInvitationList;
import io.flowcatalyst.sdk.generated.model.ApplicationUserList;
import io.flowcatalyst.sdk.generated.model.InviteApplicationUserRequest;
import io.flowcatalyst.sdk.generated.model.InviteApplicationUserResponse;
import io.flowcatalyst.sdk.generated.model.ResendInvitationRequest;
import io.flowcatalyst.sdk.generated.model.ResendInvitationResponse;
import java.time.Duration;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * The reads of events and dispatch jobs and the application user/invitation calls an
 * application's admin pages make as its own service account: paths, query encoding (plural
 * filters travel as one comma-separated value), bodies, and the typed answers.
 */
class ApplicationAdminResourcesTest {

    private StubServer server;

    @BeforeEach
    void setUp() throws Exception {
        server = new StubServer();
        server.stubToken("tok");
    }

    @AfterEach
    void tearDown() {
        server.close();
    }

    private FlowCatalystClient client() {
        return FlowCatalystClient.builder()
                .baseUrl(server.baseUrl())
                .clientCredentials("id", "secret")
                .retryDelay(Duration.ofMillis(1))
                .build();
    }

    private String lastPath() {
        return server.requests.getLast().pathAndQuery();
    }

    // ── events ───────────────────────────────────────────────────────

    @Test
    void eventsListSendsFiltersAndReadsABareArray() {
        server.on("GET", "/api/events", 200,
                "[{\"id\":\"e1\",\"type\":\"orders:sales:order:placed\",\"source\":\"orders\","
                        + "\"time\":\"2026-10-16T10:00:00.000000Z\",\"projectedAt\":\"2026-10-16T10:00:01.000000Z\"}]");

        var events = client().events().list(new EventsResource.Filters(
                "orders:sales:order:placed", null, null, "clt_1", null,
                "2026-10-16T00:00:00Z", null, List.of("a:b:c:d", "a:b:c:e"), List.of("orders"), 25, 50));

        assertEquals(1, events.size());
        assertEquals("e1", events.getFirst().getId());
        String path = lastPath();
        assertTrue(path.startsWith("/api/events?"), path);
        assertTrue(path.contains("type=orders%3Asales%3Aorder%3Aplaced"), path);
        assertTrue(path.contains("clientId=clt_1"), path);
        assertTrue(path.contains("types=a%3Ab%3Ac%3Ad%2Ca%3Ab%3Ac%3Ae"), "one comma-separated value: " + path);
        assertTrue(path.contains("applications=orders"), path);
        assertTrue(path.contains("limit=25") && path.contains("offset=50"), path);
        assertFalse(path.contains("source="), "unset filters are omitted: " + path);
    }

    @Test
    void eventsListWithNoFiltersHasNoQueryAndAnEmptyAnswerIsAnEmptyList() {
        server.on("GET", "/api/events", 200, "[]");
        assertTrue(client().events().list(null).isEmpty());
        assertEquals("/api/events", lastPath());
    }

    @Test
    void eventGetEncodesTheIdAndAnotherApplicationsEventIsA404() {
        server.on("GET", "/api/events/e/1", 200, "{\"id\":\"e/1\",\"type\":\"a:b:c:d\",\"source\":\"s\"}");
        assertEquals("e/1", client().events().get("e/1").getId());

        server.on("GET", "/api/events/other", 404, "{\"error\":\"Event_NOT_FOUND\",\"message\":\"Event not found: other\"}");
        FlowCatalystException ex = assertThrows(FlowCatalystException.class, () -> client().events().get("other"));
        assertInstanceOf(SdkError.NotFound.class, ex.error());
    }

    // ── dispatch jobs ────────────────────────────────────────────────

    @Test
    void dispatchJobReads() {
        server.on("GET", "/api/dispatch-jobs", 200,
                "[{\"id\":\"j1\",\"code\":\"orders:sales:order:placed\",\"status\":\"COMPLETED\",\"kind\":\"EVENT\","
                        + "\"targetUrl\":\"https://t.example.com\",\"mode\":\"IMMEDIATE\","
                        + "\"createdAt\":\"2026-10-16T10:00:00.000000Z\"}]");
        server.on("GET", "/api/dispatch-jobs/j1/attempts", 200,
                "[{\"attemptNumber\":1,\"success\":true,\"attemptedAt\":\"2026-10-16T10:00:02.000000Z\"}]");
        server.on("GET", "/api/dispatch-jobs/event/ev1", 200, "[]");

        var jobs = client().dispatchJobs().list(new DispatchJobsResource.Filters(
                "COMPLETED", null, null, null, "grp", null, null,
                List.of("FAILED", "PENDING"), List.of("orders:sales:order:placed"), List.of("orders"),
                "createdAt.asc", 10, 0));
        assertEquals(1, jobs.size());
        assertEquals("j1", jobs.getFirst().getId());
        String path = lastPath();
        assertTrue(path.contains("status=COMPLETED"), path);
        assertTrue(path.contains("statuses=FAILED%2CPENDING"), path);
        assertTrue(path.contains("messageGroup=grp"), path);
        assertTrue(path.contains("sort=createdAt.asc"), path);
        assertTrue(path.contains("limit=10") && path.contains("offset=0"), path);

        var attempts = client().dispatchJobs().attempts("j1");
        assertEquals(1, attempts.size());
        assertTrue(attempts.getFirst().getSuccess());

        assertTrue(client().dispatchJobs().byEvent("ev1").isEmpty());
        assertEquals("/api/dispatch-jobs/event/ev1", lastPath());
    }

    @Test
    void aCredentialBoundToNoApplicationIsRefusedWith403() {
        server.on("GET", "/api/dispatch-jobs", 403,
                "{\"error\":\"FORBIDDEN\",\"message\":\"this credential is not bound to an application\"}");
        FlowCatalystException ex = assertThrows(FlowCatalystException.class,
                () -> client().dispatchJobs().list(DispatchJobsResource.Filters.none()));
        assertInstanceOf(SdkError.Forbidden.class, ex.error());
    }

    // ── application users ────────────────────────────────────────────

    @Test
    void usersListAndGet() {
        server.on("GET", "/api/applications/orders/users", 200,
                "{\"users\":[{\"id\":\"prn_1\",\"email\":\"a@x.test\",\"name\":\"A\",\"active\":true,"
                        + "\"clientId\":\"clt_1\",\"idpType\":\"INTERNAL\",\"roles\":[\"orders:clerk\"],"
                        + "\"invitation\":\"PENDING\",\"createdAt\":\"2026-10-16T10:00:00.000000Z\"}],"
                        + "\"total\":1,\"page\":0,\"pageSize\":50}");
        server.on("GET", "/api/applications/orders/users/prn_1", 200,
                "{\"id\":\"prn_1\",\"email\":\"a@x.test\",\"name\":\"A\",\"active\":true,\"clientId\":\"clt_1\","
                        + "\"idpType\":\"INTERNAL\",\"roles\":[],\"createdAt\":\"2026-10-16T10:00:00.000000Z\"}");

        ApplicationUserList list = client().applicationUsers().list("orders", new ApplicationUsersResource.Filters(
                "ann", true, "clt_1", List.of("orders:clerk", "orders:admin"), 1, 20, "email", "desc"));
        assertEquals(1, list.getUsers().size());
        assertEquals("PENDING", list.getUsers().getFirst().getInvitation());
        assertEquals(1L, list.getTotal());
        String path = lastPath();
        assertTrue(path.contains("q=ann") && path.contains("active=true"), path);
        assertTrue(path.contains("roles=orders%3Aclerk%2Corders%3Aadmin"), path);
        assertTrue(path.contains("page=1") && path.contains("pageSize=20"), path);
        assertTrue(path.contains("sortField=email") && path.contains("sortOrder=desc"), path);

        assertEquals("prn_1", client().applicationUsers().get("orders", "prn_1").getId());
    }

    @Test
    void anotherApplicationIsNotFound() {
        // Platform F32: another application's code, an unbound credential and an unknown code
        // all answer 404, so a caller cannot enumerate applications. A missing permission is
        // still a 403.
        server.on("GET", "/api/applications/billing/users", 404,
                "{\"error\":\"Application_NOT_FOUND\",\"message\":\"Application not found\"}");
        FlowCatalystException ex = assertThrows(FlowCatalystException.class,
                () -> client().applicationUsers().list("billing", null));
        assertInstanceOf(SdkError.NotFound.class, ex.error());
    }

    @Test
    void inviteSendsTheBodyAndReturnsTheLinkOnlyWhenAsked() {
        server.on("POST", "/api/applications/orders/users", 201,
                "{\"id\":\"prn_2\",\"inviteLink\":\"https://platform.example/auth/set-password?token=abc\","
                        + "\"invitationSent\":false}");

        InviteApplicationUserResponse r = client().applicationUsers().invite("orders",
                new InviteApplicationUserRequest()
                        .email("b@x.test").name("B").clientId("clt_1").roles(List.of("clerk"))
                        .returnInviteLink(true).inviteRedirectUri("https://orders.example/welcome"));

        assertEquals("prn_2", r.getId());
        assertEquals("https://platform.example/auth/set-password?token=abc", r.getInviteLink());
        assertFalse(r.getInvitationSent());
        String body = server.requests.getLast().body();
        assertTrue(body.contains("\"roles\":[\"clerk\"]"), body);
        assertTrue(body.contains("\"returnInviteLink\":true"), body);
        assertTrue(body.contains("\"inviteRedirectUri\":\"https://orders.example/welcome\""), body);
        assertFalse(body.contains("sendInvitation"), "unset flags are omitted: " + body);
    }

    @Test
    void invitationsResendAndRevoke() {
        server.on("GET", "/api/applications/orders/invitations", 200,
                "{\"invitations\":[{\"userId\":\"prn_1\",\"email\":\"a@x.test\",\"name\":\"A\",\"clientId\":\"clt_1\","
                        + "\"roles\":[\"orders:clerk\"],\"status\":\"EXPIRED\",\"createdAt\":\"2026-10-16T10:00:00.000000Z\"}],"
                        + "\"total\":1,\"page\":0,\"pageSize\":50}");
        ApplicationInvitationList inv = client().applicationUsers().invitations("orders", "EXPIRED", 0, 50);
        assertEquals("EXPIRED", inv.getInvitations().getFirst().getStatus());
        assertNull(inv.getInvitations().getFirst().getExpiresAt(), "no live token: no expiry");
        assertTrue(lastPath().contains("status=EXPIRED"), lastPath());

        server.on("POST", "/api/applications/orders/invitations/prn_1/resend", 200,
                "{\"inviteLink\":\"https://platform.example/auth/set-password?token=def\",\"expiresAt\":\"2026-10-19T10:00:00.000000Z\"}");
        ResendInvitationResponse resent = client().applicationUsers()
                .resend("orders", "prn_1", new ResendInvitationRequest().returnInviteLink(true));
        assertTrue(resent.getInviteLink().endsWith("token=def"));
        assertTrue(server.requests.getLast().body().contains("\"returnInviteLink\":true"));

        server.on("POST", "/api/applications/orders/invitations/prn_2/resend", 200, "{}");
        client().applicationUsers().resend("orders", "prn_2", null);
        assertEquals("POST", server.requests.getLast().method());

        server.on("DELETE", "/api/applications/orders/invitations/prn_1", 204, "");
        client().applicationUsers().revoke("orders", "prn_1");
        assertEquals("DELETE", server.requests.getLast().method());
        assertEquals("/api/applications/orders/invitations/prn_1", lastPath());
    }
}
