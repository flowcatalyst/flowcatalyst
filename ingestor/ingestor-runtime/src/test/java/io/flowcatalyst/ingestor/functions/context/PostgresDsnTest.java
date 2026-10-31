package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.functions.context.PostgresDsn.DsnException;
import io.flowcatalyst.ingestor.functions.context.PostgresDsn.Target;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/// The DSN forms pgxpool.ParseConfig takes, as JDBC. No exception, string form or ignored-key report carries the password.
class PostgresDsnTest {

    static final String PW = "S3cr3t!pw";

    @Test
    void urlForm() {
        Target t = PostgresDsn.parse("postgres://app:" + PW + "@db.example.com:6543/orders?sslmode=require&application_name=fn");
        assertThat(t.jdbcUrl()).isEqualTo("jdbc:postgresql://db.example.com:6543/orders");
        assertThat(t.user()).isEqualTo("app");
        assertThat(t.password()).isEqualTo(PW);
        assertThat(t.properties()).containsEntry("sslmode", "require").containsEntry("ApplicationName", "fn");
        assertThat(t.jdbcUrl()).doesNotContain(PW);
        assertThat(t.toString()).doesNotContain(PW);
    }

    @Test
    void postgresqlSchemeAndDefaults() {
        Target t = PostgresDsn.parse("postgresql://u@h");
        assertThat(t.jdbcUrl()).isEqualTo("jdbc:postgresql://h:5432/u"); // database defaults to the user, as pgx
        assertThat(t.password()).isNull();
    }

    @Test
    void percentEncodedUserAndPassword() {
        Target t = PostgresDsn.parse("postgres://a%40b:p%3Aw%2Fd%25+x@h:5432/d%20b");
        assertThat(t.user()).isEqualTo("a@b");
        assertThat(t.password()).isEqualTo("p:w/d%+x"); // '+' is literal in userinfo
        assertThat(t.jdbcUrl()).isEqualTo("jdbc:postgresql://h:5432/d+b");
    }

    @Test
    void passwordWithAtSignInUserinfoSplitsAtTheLastAt() {
        Target t = PostgresDsn.parse("postgres://u:p@ss@h/d");
        assertThat(t.password()).isEqualTo("p@ss");
        assertThat(t.jdbcUrl()).isEqualTo("jdbc:postgresql://h:5432/d");
    }

    @Test
    void multipleHostsAndIpv6() {
        assertThat(PostgresDsn.parse("postgres://u:p@h1:1,h2:2/d?target_session_attrs=read-write").jdbcUrl())
                .isEqualTo("jdbc:postgresql://h1:1,h2:2/d");
        assertThat(PostgresDsn.parse("postgres://u:p@h1:1,h2:2/d?target_session_attrs=read-write").properties())
                .containsEntry("targetServerType", "primary");
        assertThat(PostgresDsn.parse("postgres://u:p@[::1]:7000/d").jdbcUrl()).isEqualTo("jdbc:postgresql://[::1]:7000/d");
        assertThat(PostgresDsn.parse("postgres://u:p@h1,h2/d").jdbcUrl()).isEqualTo("jdbc:postgresql://h1:5432,h2:5432/d");
    }

    @Test
    void keywordForm() {
        Target t = PostgresDsn.parse("host=db port=5433 user=app password=" + PW + " dbname=orders sslmode=verify-full connect_timeout=7");
        assertThat(t.jdbcUrl()).isEqualTo("jdbc:postgresql://db:5433/orders");
        assertThat(t.user()).isEqualTo("app");
        assertThat(t.password()).isEqualTo(PW);
        assertThat(t.properties()).containsEntry("sslmode", "verify-full").containsEntry("connectTimeout", "7").containsEntry("loginTimeout", "7");
    }

    @Test
    void keywordFormQuotedAndEscaped() {
        Target t = PostgresDsn.parse("host='db' user=app password='it\\'s a pw' dbname=x");
        assertThat(t.password()).isEqualTo("it's a pw");
        Target e = PostgresDsn.parse("host=db user=app password=a\\ b dbname=x");
        assertThat(e.password()).isEqualTo("a b");
    }

    @Test
    void keywordFormMultiHostPorts() {
        assertThat(PostgresDsn.parse("host=a,b port=1,2 user=u dbname=d").jdbcUrl()).isEqualTo("jdbc:postgresql://a:1,b:2/d");
        assertThat(PostgresDsn.parse("host=a,b port=9 user=u dbname=d").jdbcUrl()).isEqualTo("jdbc:postgresql://a:9,b:9/d");
    }

    @ParameterizedTest
    @ValueSource(strings = {"disable", "allow", "prefer", "require", "verify-ca", "verify-full"})
    void everySslModeMaps(String mode) {
        assertThat(PostgresDsn.parse("postgres://u:p@h/d?sslmode=" + mode).properties()).containsEntry("sslmode", mode);
        assertThat(PostgresDsn.parse("host=h user=u sslmode=" + mode).properties()).containsEntry("sslmode", mode);
    }

    @Test
    void pgxOnlyParametersAreIgnoredAndReportedByKeyOnly() {
        List<String> ignored = new ArrayList<>();
        Target t = PostgresDsn.parse("postgres://u:" + PW + "@h/d?pool_max_conns=50&statement_cache_capacity=TOPVALUE&sslmode=disable", ignored::add);
        assertThat(ignored).containsExactlyInAnyOrder("pool_max_conns", "statement_cache_capacity");
        assertThat(String.join(",", ignored)).doesNotContain("TOPVALUE").doesNotContain(PW);
        assertThat(t.properties()).containsOnlyKeys("sslmode");
    }

    @ParameterizedTest
    @ValueSource(strings = {
        "",
        "   ",
        "not a dsn",
        "postgres://u:" + PW + "@/d",                       // no host
        "postgres://u:" + PW + "@%2Fvar%2Frun%2Fpostgresql/d", // unix socket
        "postgres://u:" + PW + "@h:notaport/d",
        "postgres://u:" + PW + "@h:99999/d",
        "postgres://u:" + PW + "@h:5432/d?sslmode=bogus",
        "postgres://u:" + PW + "@h:5432/d?connect_timeout=soon",
        "postgres://u:" + PW + "@h:5432/d?target_session_attrs=whatever",
        "postgres://u:" + PW + "@h:5432/d%zz",
        "postgres://u:" + PW + "@[::1/d",
        "postgres://:" + PW + "@h/d",                        // no user
        "host=h password=" + PW,                               // no user
        "user=u password=" + PW,                               // no host
        "host=/var/run/postgresql user=u password=" + PW,
        "host=h user=u password='" + PW,                       // unterminated quote
        "host=a,b port=1,2,3 user=u password=" + PW
    })
    void rejectedWithAClearMessageThatCarriesNoPassword(String dsn) {
        assertThatThrownBy(() -> PostgresDsn.parse(dsn))
                .isInstanceOf(DsnException.class)
                .satisfies(e -> {
                    assertThat(e.getMessage()).isNotBlank().doesNotContain(PW);
                    assertThat(e.toString()).doesNotContain(PW);
                    assertThat(e.getCause()).isNull();
                });
    }
}
