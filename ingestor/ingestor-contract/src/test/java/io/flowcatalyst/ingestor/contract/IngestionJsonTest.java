package io.flowcatalyst.ingestor.contract;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.util.List;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.DeserializationFeature;
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
  void anHttpSourceKeepsWhatItAcceptsAndOneWrittenWithoutReadsAsTakingAnything() {
    Source.Http.Accept accept = new Source.Http.Accept(List.of("application/xml", "text/xml"), Source.Http.Body.XML);
    Source source = new Source.Http(new HttpAuth.Basic("acme", new SecretRef("http")), null, accept);

    String json = mapper.writeValueAsString(source);

    assertThat(json).contains("\"accept\":{\"contentTypes\":[\"application/xml\",\"text/xml\"],\"body\":\"XML\"}");
    assertThat(mapper.readValue(json, Source.class)).isEqualTo(source);
    Source.Http old =
        (Source.Http) mapper.readValue("{\"kind\":\"http\",\"auth\":{\"kind\":\"basic\",\"username\":\"acme\",\"password\":{\"name\":\"http\"}}}", Source.class);
    assertThat(old.accept()).isNull();
    Source.Http bare = (Source.Http) mapper.readValue(
        "{\"kind\":\"http\",\"auth\":{\"kind\":\"basic\",\"username\":\"acme\",\"password\":{\"name\":\"http\"}},\"accept\":{}}", Source.class);
    assertThat(bare.accept().types()).isEmpty();
    assertThat(bare.accept().check()).isEqualTo(Source.Http.Body.NONE);
  }

  @Test
  void anEventTargetAsThePlatformWritesItIsRead() {
    // The platform leaves pool and mode out of an event target and adds kind and wrapNonJson, which this
    // record does not have; the runtime reads platform answers ignoring unknown properties (PlatformApi).
    ObjectMapper lenient = JsonMapper.builder().disable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();

    Target target =
        lenient.readValue("{\"kind\":\"EVENT\",\"code\":\"bevco:yard:trip:received\",\"batchSize\":100,\"wrapNonJson\":true}", Target.class);

    assertThat(target.code()).isEqualTo("bevco:yard:trip:received");
    assertThat(target.batchSize()).isEqualTo(100);
    assertThat(target.pool()).isNull();
    assertThat(target.mode()).isNull();
  }

  @Test
  void aJobsAnswerWithoutAKindIsOneOfDispatchJobs() {
    JobsAnswer older = mapper.readValue("{\"created\":1,\"results\":[{\"index\":0,\"id\":\"djb_1\"}]}", JobsAnswer.class);
    JobsAnswer events =
        mapper.readValue("{\"kind\":\"EVENT\",\"created\":1,\"results\":[{\"index\":0,\"id\":\"evt_1\",\"existing\":true}]}", JobsAnswer.class);

    assertThat(older.kind()).isNull();
    assertThat(older.events()).isFalse();
    assertThat(events.events()).isTrue();
    assertThat(events.results().getFirst().existing()).isTrue();
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
