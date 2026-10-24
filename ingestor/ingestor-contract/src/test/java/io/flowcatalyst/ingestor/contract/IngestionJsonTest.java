package io.flowcatalyst.ingestor.contract;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

/** The sealed types keep their kind across JSON. */
class IngestionJsonTest {

  private final ObjectMapper mapper = JsonMapper.builder().build();

  @Test
  void anIngestionSurvivesARoundTrip() {
    Ingestion ingestion =
        new Ingestion(
            "ing_1",
            3,
            "clt_acme",
            "acme",
            "logistics",
            "orders-sftp",
            "Acme order files",
            new Source.Sftp(
                "sftp.acme.example", 22, "/out", "ingest", new SecretRef("sftp"), null, "*.csv", Duration.ofMinutes(1)),
            new Split.Csv(true),
            new Target(
                "logistics:orders:file:map",
                "https://functions.example/acme/order-mapper",
                "acme-pool",
                "acme-fn",
                null,
                Target.Mode.BLOCK_ON_ERROR,
                100),
            false);

    String json = mapper.writeValueAsString(ingestion);

    assertThat(json).contains("\"kind\":\"sftp\"").contains("\"kind\":\"csv\"");
    assertThat(mapper.readValue(json, Ingestion.class)).isEqualTo(ingestion);
  }

  @Test
  void anHttpSourceKeepsItsAuthKind() {
    Source source = new Source.Http(new HttpAuth.Basic("acme", new SecretRef("http")), new RateLimit(10, Duration.ofSeconds(1)));

    String json = mapper.writeValueAsString(source);

    assertThat(json).contains("\"kind\":\"http\"").contains("\"kind\":\"basic\"");
    assertThat(mapper.readValue(json, Source.class)).isEqualTo(source);
  }

  @Test
  void aDatabasePollKeepsHowItTracksRows() {
    Source byWatermark =
        new Source.DatabasePoll(
            "jdbc:postgresql://db.acme.example/erp", "ingest", new SecretRef("db"), "erp.orders",
            new Source.Track.Watermark("id", Source.Track.Watermark.Type.NUMBER), 500, Duration.ofSeconds(30));
    Source byStatus =
        new Source.DatabasePoll(
            "jdbc:postgresql://db.acme.example/erp", "ingest", new SecretRef("db"), "erp.orders",
            new Source.Track.Status("processed", "id", "0", "1"), 500, Duration.ofSeconds(30));

    assertThat(mapper.writeValueAsString(byWatermark)).contains("\"kind\":\"database-poll\"").contains("\"kind\":\"watermark\"");
    assertThat(mapper.readValue(mapper.writeValueAsString(byWatermark), Source.class)).isEqualTo(byWatermark);
    assertThat(mapper.readValue(mapper.writeValueAsString(byStatus), Source.class)).isEqualTo(byStatus);
    assertThat(byStatus.singleOwner()).isTrue();
  }
}
