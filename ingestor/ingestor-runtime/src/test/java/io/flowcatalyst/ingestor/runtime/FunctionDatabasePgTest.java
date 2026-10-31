package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.database.PostgresFixture;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.util.List;
import java.util.Map;
import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIf;

/// `select 1` through `ctx.dataSource(name)` against a real PostgreSQL. Skipped without Docker.
@EnabledIf("io.flowcatalyst.ingestor.runtime.Docker#available")
class FunctionDatabasePgTest {

  @Test
  void selectOneThroughTheContext() throws Exception {
    try (PostgresFixture pg = PostgresFixture.start();
        HttpClients clients = HttpClients.create();
        DbPools pools = new DbPools(DbPools.Limits.DEFAULTS, Clock.systemUTC())) {
      String hostAndDb = pg.jdbcUrl().substring("jdbc:postgresql://".length());
      String dsn = "postgres://" + enc(pg.username()) + ":" + enc(pg.password()) + "@" + hostAndDb;
      Control.FunctionDoc doc =
          new Control.FunctionDoc("fn_1", "app.hello", "app_1", null, null, false, null, Map.of(), Map.of(), Map.of("main", dsn), List.of());
      Describe describe =
          new Describe(1, List.of(), List.of(), List.of(), List.of("main"), List.of(), List.of(), List.of(), new Describe.Jvm("a.B"));
      DataSource ds =
          new ContextFactory(new OutboundClients(clients), Clock.systemUTC(), new FakeControlPlane(), pools).build(doc, 1, describe).dataSource("main");
      try (var c = ds.getConnection();
          var st = c.createStatement();
          var rs = st.executeQuery("select 1")) {
        assertThat(rs.next()).isTrue();
        assertThat(rs.getInt(1)).isEqualTo(1);
      }
      assertThat(pools.size()).isEqualTo(1);
    }
  }

  private static String enc(String s) {
    return URLEncoder.encode(s, StandardCharsets.UTF_8).replace("+", "%20");
  }
}
