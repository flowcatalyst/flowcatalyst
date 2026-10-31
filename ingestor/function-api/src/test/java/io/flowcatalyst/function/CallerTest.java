package io.flowcatalyst.function;

import java.lang.reflect.RecordComponent;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// `Caller`'s three cases and `Principal`'s helpers. The cases mirror
/// `clients/fn-ts/src/caller.test.ts` one for one, plus the `id:code`
/// `applications` entry rule the token actually carries.
class CallerTest {

    @Test
    void platformAndAnonymousAreSharedInstances() {
        assertThat(Caller.Platform.INSTANCE).isInstanceOf(Caller.class);
        assertThat(Caller.Anonymous.INSTANCE).isInstanceOf(Caller.class);
        assertThat(new Caller.Platform()).isEqualTo(Caller.Platform.INSTANCE);
        assertThat(new Caller.Anonymous()).isEqualTo(Caller.Anonymous.INSTANCE);
    }

    @Test
    void aSwitchOverTheSealedTypeIsExhaustive() {
        for (Caller caller : List.<Caller>of(Caller.Platform.INSTANCE, Caller.Anonymous.INSTANCE, principal().build())) {
            String kind = switch (caller) {
                case Caller.Platform p -> "webhook";
                case Caller.Principal p -> "principal";
                case Caller.Anonymous a -> "anonymous";
            };
            assertThat(kind).isNotEmpty();
        }
    }

    @Test
    void nullFieldsDefaultLikeTheOtherSdks() {
        Caller.Principal p = new Caller.Principal(null, null, null, null, null, null, null, false, null);
        assertThat(p.id()).isEmpty();
        assertThat(p.type()).isEmpty();
        assertThat(p.tier()).isEmpty();
        assertThat(p.clients()).isEmpty();
        assertThat(p.clientCodes()).isEmpty();
        assertThat(p.roles()).isEmpty();
        assertThat(p.applications()).isEmpty();
        assertThat(p.permissions()).isEmpty();
        assertThat(p.allApplications()).isFalse();
    }

    @Test
    void principalPermissionsAreIndependentOfTheSetPassedIn() {
        Set<String> permissions = new HashSet<>(Set.of("read"));
        Caller.Principal principal = principal().permissions(permissions).build();
        permissions.add("write");
        assertThat(principal.permissions()).containsExactly("read");
    }

    @Test
    void principalCollectionsAreUnmodifiable() {
        Caller.Principal principal = principal().permissions(Set.of("read")).roles(List.of("r")).build();
        assertThatThrownBy(() -> principal.permissions().add("write")).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> principal.roles().add("x")).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> principal.clientCodes().add("x")).isInstanceOf(UnsupportedOperationException.class);
    }

    // ---- isAnchor / hasRole / clients (fn-ts: caller.test.ts) ----

    @Test
    void isAnchorOnlyWhenTierIsAnchor() {
        assertThat(principal().tier("ANCHOR").build().isAnchor()).isTrue();
        assertThat(principal().tier("CLIENT").build().isAnchor()).isFalse();
        assertThat(principal().tier(null).build().isAnchor()).isFalse();
    }

    @Test
    void hasRoleIsExactOnlyNeverWildcarded() {
        Caller.Principal p = principal().roles(List.of("operant:admin", "billing:viewer")).build();
        assertThat(p.hasRole("operant:admin")).isTrue();
        assertThat(p.hasRole("operant:*")).isFalse();
    }

    @Test
    void canAccessClientAnAnchorAlwaysPasses() {
        assertThat(principal().tier("ANCHOR").build().canAccessClient("any-client")).isTrue();
    }

    @Test
    void canAccessClientIsScopedToTheExplicitClientsList() {
        Caller.Principal p = principal().tier("CLIENT").clients(List.of("clt_a")).build();
        assertThat(p.canAccessClient("clt_a")).isTrue();
        assertThat(p.canAccessClient("clt_z")).isFalse();
    }

    @Test
    void canAccessClientCodeAnchorAndExactMatchOnly() {
        assertThat(principal().tier("ANCHOR").build().canAccessClientCode("bevco")).isTrue();
        Caller.Principal p = principal().tier("CLIENT").clientCodes(List.of("bevco")).build();
        assertThat(p.canAccessClientCode("bevco")).isTrue();
        assertThat(p.canAccessClientCode("bev")).isFalse();
        assertThat(principal().tier("CLIENT").build().canAccessClientCode("bevco")).isFalse();
    }

    // ---- canAccessApplication, including the id:code rule ----

    @Test
    void canAccessApplicationAllApplicationsShortCircuits() {
        assertThat(principal().allApplications(true).build().canAccessApplication("app_1")).isTrue();
        Caller.Principal scoped = principal().applications(List.of("app_1")).build();
        assertThat(scoped.canAccessApplication("app_1")).isTrue();
        assertThat(scoped.canAccessApplication("app_2")).isFalse();
    }

    @Test
    void canAccessApplicationMatchesTheIdHalfOfAnIdCodePair() {
        Caller.Principal p = principal().applications(List.of("app_1:orders", "app_2:billing")).build();
        assertThat(p.canAccessApplication("app_1")).isTrue();
        assertThat(p.canAccessApplication("app_2")).isTrue();
        assertThat(p.canAccessApplication("app_3")).isFalse();
    }

    @Test
    void canAccessApplicationNeverMatchesTheCodeHalf() {
        Caller.Principal p = principal().applications(List.of("app_1:orders")).build();
        assertThat(p.canAccessApplication("orders")).isFalse();
    }

    @Test
    void canAccessApplicationStillMatchesAWholeEntryExactlyLikeFnTs() {
        // fn-ts compares the id against the raw entries; a bare id entry is the older token form.
        assertThat(principal().applications(List.of("app_1")).build().canAccessApplication("app_1")).isTrue();
    }

    @Test
    void canAccessApplicationSplitsOnTheFirstColonOnly() {
        Caller.Principal p = principal().applications(List.of("app_1:co:de")).build();
        assertThat(p.canAccessApplication("app_1")).isTrue();
        assertThat(p.canAccessApplication("app_1:co")).isFalse();
    }

    @Test
    void canAccessApplicationCodeMatchesTheCodeHalfABareCodeOrStar() {
        Caller.Principal p = principal().applications(List.of("app_1:bevco-integration", "orders")).build();
        assertThat(p.canAccessApplicationCode("bevco-integration")).isTrue();
        assertThat(p.canAccessApplicationCode("orders")).isTrue();
        assertThat(p.canAccessApplicationCode("app_1")).isFalse();
        assertThat(p.canAccessApplicationCode("merensky-integration")).isFalse();
        assertThat(principal().applications(List.of("*")).build().canAccessApplicationCode("x")).isTrue();
        assertThat(principal().allApplications(true).build().canAccessApplicationCode("x")).isTrue();
    }

    @Test
    void canAccessApplicationStarEntryMeansEveryApplication() {
        assertThat(principal().applications(List.of("*")).build().canAccessApplication("anything")).isTrue();
    }

    @Test
    void canAccessApplicationNoEntryNoAccess() {
        assertThat(principal().build().canAccessApplication("app_1")).isFalse();
        assertThat(principal().applications(List.of(":code")).build().canAccessApplication("")).isFalse();
    }

    // ---- hasPermission (fn-ts: caller.test.ts) ----

    @ParameterizedTest(name = "[{index}] held={0} required={1} -> {2}")
    @CsvSource({
            "platform:function:view, platform:function:view, true",
            "platform:function:view, platform:function:manage, false",
            "platform:messaging:*:*, platform:messaging:queue:read, true",
            "platform:*:*:*, platform:messaging:queue:read, true",
            "platform:*, platform:messaging:queue:read, false",
            "platform:*:*:*:*, platform:messaging:queue, false",
            "platform:*:queue:read, platform:messaging:queue:read, true",
            "platform:*:queue:read, platform:messaging:topic:read, false",
            "a:b:c:d, a:b:c:e, false",
            "a:b:c, a:b:c:d, false",
            "a:b:c:d:e, a:b:c:d, false",
            "*:*:*:*, a:b:c:d, true",
            "a:b:*:d, a:b:c:d, true",
    })
    void hasPermissionMatchesSegmentwise(String held, String required, boolean expected) {
        assertThat(principal().permissions(Set.of(held)).build().hasPermission(required)).isEqualTo(expected);
    }

    @Test
    void hasPermissionEmptyPermissions() {
        assertThat(principal().permissions(Set.of()).build().hasPermission("platform:function:view")).isFalse();
    }

    @Test
    void hasPermissionOneOfSeveralHeld() {
        Caller.Principal p = principal().permissions(Set.of("a:b", "platform:function:*")).build();
        assertThat(p.hasPermission("platform:function:view")).isTrue();
    }

    @Test
    void hasPermissionIsFalseForNullRequired() {
        assertThat(principal().permissions(Set.of("a:b:c:d")).build().hasPermission(null)).isFalse();
    }

    @Test
    void hasAnyAndHasAllPermissions() {
        Caller.Principal p = principal().permissions(Set.of("a:b:c:read", "a:b:c:write")).build();
        assertThat(p.hasAnyPermission("a:b:c:delete", "a:b:c:read")).isTrue();
        assertThat(p.hasAnyPermission("a:b:c:delete", "a:b:c:purge")).isFalse();
        assertThat(p.hasAllPermissions("a:b:c:read", "a:b:c:write")).isTrue();
        assertThat(p.hasAllPermissions("a:b:c:read", "a:b:c:delete")).isFalse();
    }

    @Test
    void principalCarriesExactlyTheAbiCallerFieldsAndNoEmailOrName() {
        Set<String> names = new HashSet<>();
        for (RecordComponent c : Caller.Principal.class.getRecordComponents()) {
            names.add(c.getName());
        }
        assertThat(names).containsExactlyInAnyOrder("id", "type", "tier", "clients", "clientCodes", "roles",
                "applications", "allApplications", "permissions");
    }

    private static PrincipalBuilder principal() {
        return new PrincipalBuilder();
    }

    private static final class PrincipalBuilder {
        private String tier = "CLIENT";
        private List<String> clients = new ArrayList<>();
        private List<String> clientCodes = new ArrayList<>();
        private List<String> roles = new ArrayList<>();
        private List<String> applications = new ArrayList<>();
        private boolean allApplications;
        private Set<String> permissions = Set.of();

        PrincipalBuilder tier(String v) { tier = v; return this; }
        PrincipalBuilder clients(List<String> v) { clients = v; return this; }
        PrincipalBuilder clientCodes(List<String> v) { clientCodes = v; return this; }
        PrincipalBuilder roles(List<String> v) { roles = v; return this; }
        PrincipalBuilder applications(List<String> v) { applications = v; return this; }
        PrincipalBuilder allApplications(boolean v) { allApplications = v; return this; }
        PrincipalBuilder permissions(Set<String> v) { permissions = v; return this; }

        Caller.Principal build() {
            return new Caller.Principal("prn_1", "USER", tier, clients, clientCodes, roles, applications,
                    allApplications, permissions);
        }
    }
}
