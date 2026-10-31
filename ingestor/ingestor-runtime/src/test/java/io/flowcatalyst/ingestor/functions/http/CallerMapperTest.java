package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.auth.Principal;
import io.flowcatalyst.framework.context.PrincipalId;
import io.flowcatalyst.framework.context.PrincipalKind;
import io.flowcatalyst.function.Caller;
import java.util.List;
import java.util.Set;
import org.junit.jupiter.api.Test;

/// The mapping of a verified token's principal to the function API's `Caller.Principal`, against the Go runner's
/// (`internal/functions/runner/auth_test.go`): clients reduced to ids, codes kept, `*` stays, permissions are the
/// scope.
class CallerMapperTest {

    private static Principal principal(List<String> clients, List<String> applications) {
        return new Principal(
                new PrincipalId("prn_1"), PrincipalKind.SERVICE, "name", "e@x", "CLIENT", "clt_a", clients, applications,
                List.of("orders:admin"), Set.of("orders:fn:order:create", "platform:function:version:*"));
    }

    @Test
    void clientsAreIdsAndCodesAreIdentifiers() {
        Caller.Principal c = CallerMapper.principal(principal(List.of("clt_a:bevco", "clt_b", "*", "clt_c:"), List.of()));

        assertThat(c.clients()).containsExactly("clt_a", "clt_b", "*", "clt_c");
        assertThat(c.clientCodes()).containsExactly("bevco", "*");
    }

    @Test
    void theFieldsAreTheTokens() {
        Caller.Principal c = CallerMapper.principal(principal(List.of("clt_a:bevco"), List.of("app_1:orders")));

        assertThat(c.id()).isEqualTo("prn_1");
        assertThat(c.type()).isEqualTo("SERVICE");
        assertThat(c.tier()).isEqualTo("CLIENT");
        assertThat(c.roles()).containsExactly("orders:admin");
        assertThat(c.applications()).containsExactly("app_1:orders");
        assertThat(c.allApplications()).isFalse();
        assertThat(c.permissions()).containsExactlyInAnyOrder("orders:fn:order:create", "platform:function:version:*");
        // the helpers of the API restate the platform SDK's
        assertThat(c.hasPermission("platform:function:version:invoke")).isTrue();
        assertThat(c.canAccessApplicationCode("orders")).isTrue();
        assertThat(c.canAccessClientCode("bevco")).isTrue();
        assertThat(c.canAccessClientCode("other")).isFalse();
    }

    @Test
    void theStarApplicationIsAllApplications() {
        assertThat(CallerMapper.principal(principal(List.of(), List.of("*"))).allApplications()).isTrue();
    }

    @Test
    void anAbsentTierIsEmpty() {
        Principal p = new Principal(new PrincipalId("prn_1"), PrincipalKind.USER, null, null, null, null, List.of(), List.of(), List.of(), Set.of());
        Caller.Principal c = CallerMapper.principal(p);
        assertThat(c.tier()).isEmpty();
        assertThat(c.type()).isEqualTo("USER");
    }

    @Test
    void reachIsTheGoRule() {
        Caller.Principal anchor = new Caller.Principal("p", "USER", "ANCHOR", List.of(), List.of(), List.of(), List.of(), false, Set.of());
        Caller.Principal client = new Caller.Principal("p", "USER", "CLIENT", List.of("clt_a"), List.of("a"), List.of(), List.of(), false, Set.of());
        Caller.Principal star = new Caller.Principal("p", "USER", "PARTNER", List.of("*"), List.of("*"), List.of(), List.of(), false, Set.of());

        assertThat(CallerMapper.canReach(anchor, null)).as("anchors reach everything, even a platform-owned function").isTrue();
        assertThat(CallerMapper.canReach(client, null)).as("platform-owned functions are anchor-only").isFalse();
        assertThat(CallerMapper.canReach(client, "clt_a")).isTrue();
        assertThat(CallerMapper.canReach(client, "clt_b")).isFalse();
        assertThat(CallerMapper.canReach(star, "clt_b")).isTrue();
    }
}
