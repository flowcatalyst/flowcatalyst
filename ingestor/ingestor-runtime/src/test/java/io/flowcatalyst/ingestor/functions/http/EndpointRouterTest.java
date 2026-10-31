package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.functions.control.Describe;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;

/// Endpoint matching with the semantics of Go's ServeMux (`internal/functions/abi/router.go`).
class EndpointRouterTest {

    private static Describe.Endpoint ep(String method, String path) {
        return new Describe.Endpoint(method, path, Describe.AUTH_NONE, null, null, null);
    }

    private static EndpointRouter router(Describe.Endpoint... endpoints) {
        return new EndpointRouter(List.of(endpoints));
    }

    private static EndpointRouter.Matched.Found found(EndpointRouter.Matched m) {
        assertThat(m).isInstanceOf(EndpointRouter.Matched.Found.class);
        return (EndpointRouter.Matched.Found) m;
    }

    @Test
    void literalAndWildcardSegments() {
        EndpointRouter r = router(ep("GET", "/hello/{name}"));

        EndpointRouter.Matched.Found f = found(r.match("GET", "/hello/ann"));
        assertThat(f.index()).isZero();
        assertThat(f.pathParams()).isEqualTo(Map.of("name", "ann"));
        assertThat(r.match("GET", "/hello/")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
        assertThat(r.match("GET", "/hello")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
        assertThat(r.match("GET", "/hello/ann/more")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
    }

    @Test
    void aPathSomeEndpointMatchesWithAnotherMethodIs405() {
        EndpointRouter r = router(ep("GET", "/a"), ep("POST", "/b"));

        assertThat(r.match("POST", "/a")).isInstanceOf(EndpointRouter.Matched.MethodNotAllowed.class);
        assertThat(r.match("GET", "/b")).isInstanceOf(EndpointRouter.Matched.MethodNotAllowed.class);
        assertThat(r.match("GET", "/c")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
    }

    @Test
    void anEndpointWithoutAMethodTakesEveryMethod() {
        EndpointRouter r = router(ep("", "/any"));
        for (String m : new String[] {"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}) {
            found(r.match(m, "/any"));
        }
    }

    @Test
    void aGetPatternServesHead() {
        found(router(ep("GET", "/a")).match("HEAD", "/a"));
        assertThat(router(ep("POST", "/a")).match("HEAD", "/a")).isInstanceOf(EndpointRouter.Matched.MethodNotAllowed.class);
    }

    @Test
    void theMostSpecificPatternWins() {
        EndpointRouter r = router(ep("GET", "/files/{name}"), ep("GET", "/files/latest"), ep("GET", "/files/{path...}"));

        assertThat(found(r.match("GET", "/files/latest")).index()).as("a literal beats a wildcard").isEqualTo(1);
        assertThat(found(r.match("GET", "/files/x")).index()).as("a single wildcard beats a rest wildcard").isZero();
        EndpointRouter.Matched.Found deep = found(r.match("GET", "/files/a/b/c"));
        assertThat(deep.index()).isEqualTo(2);
        assertThat(deep.pathParams()).isEqualTo(Map.of("path", "a/b/c"));
    }

    @Test
    void aPatternWithAMethodBeatsOneWithout() {
        EndpointRouter r = router(ep("", "/x"), ep("POST", "/x"));
        assertThat(found(r.match("POST", "/x")).index()).isEqualTo(1);
        assertThat(found(r.match("GET", "/x")).index()).isZero();
    }

    @Test
    void aTrailingSlashIsASubtreeAndDollarIsExact() {
        EndpointRouter r = router(ep("GET", "/static/"), ep("GET", "/{$}"));

        found(r.match("GET", "/static/"));
        found(r.match("GET", "/static/css/site.css"));
        assertThat(found(r.match("GET", "/")).index()).isEqualTo(1);
        assertThat(r.match("GET", "/other")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
    }

    @Test
    void theRestWildcardMayBeEmpty() {
        EndpointRouter.Matched.Found f = found(router(ep("GET", "/f/{rest...}")).match("GET", "/f/"));
        assertThat(f.pathParams()).isEqualTo(Map.of("rest", ""));
    }

    @Test
    void anUncleanPathMatchesNothing() {
        EndpointRouter r = router(ep("GET", "/a/{x}"));
        assertThat(r.match("GET", "/a//b")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
        assertThat(r.match("GET", "/a/../b")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
        assertThat(r.match("GET", "a/b")).isInstanceOf(EndpointRouter.Matched.NoRoute.class);
    }

    @Test
    void aMalformedPatternIsRefused() {
        assertThatThrownBy(() -> router(ep("GET", "/a{b}c"))).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> router(ep("GET", "/{a...}/b"))).isInstanceOf(IllegalArgumentException.class);
    }
}
