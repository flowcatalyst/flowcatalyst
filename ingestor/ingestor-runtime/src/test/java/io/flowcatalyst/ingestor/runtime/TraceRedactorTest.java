package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.Json;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;

/** What a trace may carry: credentials by name, by position and by value never leave the runtime. */
class TraceRedactorTest {

  static final String R = "[redacted]";

  final TraceRedactor plain = TraceRedactor.withoutSecrets();

  private static byte[] bytes(String text) {
    return text.getBytes(StandardCharsets.UTF_8);
  }

  @Test
  void headersWithACredentialNameAreRedactedAndTheRestKept() {
    Map<String, List<String>> out =
        plain.headers(
            Map.of(
                "Authorization", List.of("Bearer abc"),
                "X-Api-Key", List.of("k"),
                "Set-Cookie", List.of("a=1", "b=2"),
                "Proxy-Authorization", List.of("Basic xyz"),
                "Accept", List.of("application/json")));

    assertThat(out.get("Authorization")).containsExactly(R);
    assertThat(out.get("X-Api-Key")).containsExactly(R);
    assertThat(out.get("Set-Cookie")).containsExactly(R, R);
    assertThat(out.get("Proxy-Authorization")).containsExactly(R);
    assertThat(out.get("Accept")).containsExactly("application/json");
  }

  @Test
  void aUrlLosesItsUserInfoAndEveryQueryValueButPagingAndTimeWindows() {
    String url =
        plain.url(URI.create("https://user:pw@api.example:8443/v1/items?access_token=A&page=2&X-Amz-Signature=x&code=9&name=Bob&sessionId=s&sig=q&Limit=50&since=2026-10-30"));

    assertThat(url)
        .isEqualTo(
            "https://api.example:8443/v1/items?access_token=" + R + "&page=2&X-Amz-Signature=" + R + "&code=" + R + "&name=" + R + "&sessionId=" + R + "&sig=" + R
                + "&Limit=50&since=2026-10-30");
    assertThat(url).doesNotContain("user").doesNotContain("pw@");
  }

  @Test
  void aLoginByQueryKeepsItsNamesButNotItsValuesWhateverTheyAreCalled() {
    // CTrack's login: GET .../Membership/Login?username=&password=; another API's ?user=&pwd=.
    assertThat(plain.url(URI.create("https://ct.example/DCTTPIEngine/TPIEngine/REST/Membership/Login?username=fleet&password=pw-1")))
        .isEqualTo("https://ct.example/DCTTPIEngine/TPIEngine/REST/Membership/Login?username=" + R + "&password=" + R);
    assertThat(plain.url(URI.create("https://h/login?user=fleet&pwd=pw-1&flag"))).isEqualTo("https://h/login?user=" + R + "&pwd=" + R + "&flag");
  }

  @Test
  void anAnswerHandsOutTheCredentialsInItsCredentialNamedFields() {
    String session = "6f1c0d2e-9a4b-4c3d-8e7f-0a1b2c3d4e5f";
    String login = "{\"ErrorCode\":0,\"SessionToken\":\"" + session + "\",\"SessionDurationMinutes\":60,"
        + "\"nested\":{\"access_token\":\"abcdefghijklmnopqrstuvwxyz\"},\"items\":[{\"refresh_token\":\"0123456789abcdefXYZ\"}],"
        + "\"sortKey\":\"date\",\"token\":\"short\",\"session\":\"has a space in it, so not one\",\"tokenCount\":12345678901234567}";

    assertThat(TraceRedactor.credentialsIn(bytes(login), "application/json"))
        .containsExactlyInAnyOrder(session, "abcdefghijklmnopqrstuvwxyz", "0123456789abcdefXYZ");
    assertThat(TraceRedactor.credentialsIn(bytes(login), null)).contains(session);
    assertThat(TraceRedactor.credentialsIn(bytes("<a SessionToken=\"" + session + "\"/>"), "application/xml")).isEmpty();
    assertThat(TraceRedactor.credentialsIn(bytes("{not json"), "application/json")).isEmpty();
    assertThat(TraceRedactor.credentialsIn(null, "application/json")).isEmpty();

    // Once learned, the session is taken out of a later call's path, which the metadata view shows.
    TraceRedactor redactor = new TraceRedactor(List.of(session));
    URI vehicles = URI.create("https://ct.example/DCTTPIEngine/TPIEngine/REST/Vehicles/" + session + "/GetVehicles");
    assertThat(redactor.address(vehicles)).isEqualTo("https://ct.example/DCTTPIEngine/TPIEngine/REST/Vehicles/" + R + "/GetVehicles");
    assertThat(redactor.url(vehicles)).doesNotContain(session);
  }

  @Test
  void aUrlWithoutAQueryIsItsAddress() {
    assertThat(plain.url(URI.create("http://h/p?"))).startsWith("http://h/p");
    assertThat(plain.address(URI.create("https://h:1/p/q?token=1#frag"))).isEqualTo("https://h:1/p/q");
  }

  @Test
  void jsonFieldsAreRedactedAtAnyDepthWhateverTheirValue() throws Exception {
    byte[] body =
        bytes(
            "{\"name\":\"x\",\"auth\":{\"user\":\"u\",\"password\":\"p\"},\"items\":[{\"id\":1,\"refresh_token\":\"r\"},{\"id\":2,\"assertion\":\"a\",\"credential\":{\"n\":1}}],\"private_key\":\"-----BEGIN\"}");

    Map<String, Object> out = plain.body(body, "application/json; charset=utf-8");

    JsonNode json = Json.mapper().readTree((String) out.get("body"));
    assertThat(json.get("name").asString()).isEqualTo("x");
    // "auth" matches the header rule: the whole object goes.
    assertThat(json.get("auth").asString()).isEqualTo(R);
    assertThat(json.get("items").get(0).get("id").asInt()).isEqualTo(1);
    assertThat(json.get("items").get(0).get("refresh_token").asString()).isEqualTo(R);
    assertThat(json.get("items").get(1).get("assertion").asString()).isEqualTo(R);
    assertThat(json.get("items").get(1).get("credential").asString()).isEqualTo(R);
    assertThat(json.get("private_key").asString()).isEqualTo(R);
    assertThat(out.get("bodyEncoding")).isEqualTo("utf8");
    assertThat(out.get("bodyBytes")).isEqualTo((long) body.length);
    assertThat(out.get("bodyTruncated")).isEqualTo(false);
  }

  @Test
  void aFormBodyIsRedactedByFieldName() {
    Map<String, Object> out =
        plain.body(bytes("grant_type=password&username=u%40x.com&password=p%21&client_id=c&client_secret=s&scope=read"), "application/x-www-form-urlencoded");

    assertThat(out.get("body"))
        .isEqualTo("grant_type=password&username=u%40x.com&password=" + R + "&client_id=c&client_secret=" + R + "&scope=read");
  }

  @Test
  void aTokenCallShowsWhatWasAskedAndNotTheCredentialsEitherWay() throws Exception {
    // The MiX-like login: a form request and a JSON answer.
    Map<String, Object> request =
        plain.body(bytes("grant_type=password&username=fleet&password=hunter2&scope=offline_access&client_id=abc&client_secret=shh"), "application/x-www-form-urlencoded");
    Map<String, Object> response =
        plain.body(bytes("{\"access_token\":\"eyJhbGciOi\",\"token_type\":\"Bearer\",\"expires_in\":3600,\"refresh_token\":\"rrr\",\"scope\":\"offline_access\"}"), "application/json");

    assertThat((String) request.get("body")).contains("grant_type=password", "username=fleet", "scope=offline_access", "client_id=abc").doesNotContain("hunter2", "shh");
    JsonNode answer = Json.mapper().readTree((String) response.get("body"));
    assertThat(answer.get("access_token").asString()).isEqualTo(R);
    // "token_type" holds "token", so the rule takes its value too: a rule by name does not weigh what the value is.
    assertThat(answer.get("token_type").asString()).isEqualTo(R);
    assertThat(answer.get("expires_in").asInt()).isEqualTo(3600);
    assertThat(answer.get("scope").asString()).isEqualTo("offline_access");
  }

  @Test
  void jsonWithoutAContentTypeIsFoundByItsShape() {
    Map<String, Object> out = plain.body(bytes("  {\"password\":\"p\",\"a\":1}"), null);

    assertThat((String) out.get("body")).contains(R).doesNotContain("\"p\"");
  }

  @Test
  void aBodyThatIsNeitherJsonNorAFormIsKeptAsIs() {
    Map<String, Object> out = plain.body(bytes("<a><password>p</password></a>"), "application/xml");

    assertThat(out.get("body")).isEqualTo("<a><password>p</password></a>");
  }

  @Test
  void jsonThatDoesNotParseIsRedactedByPattern() {
    Map<String, Object> out = plain.body(bytes("{\"user\":\"u\",\"password\":\"abc"), "application/json");

    assertThat((String) out.get("body")).contains("\"user\":\"u\"").doesNotContain("abc").contains(R);
  }

  @Test
  void theSecretValuesFetchedForTheIngestionAreReplacedWhereverTheyAppear() {
    TraceRedactor redactor = new TraceRedactor(List.of("s3cr3t-value", "a b&c"));

    assertThat(redactor.text("x s3cr3t-value y")).isEqualTo("x " + R + " y");
    assertThat(redactor.url(URI.create("https://h/bot-s3cr3t-value/get?z=s3cr3t-value&u=a+b%26c"))).doesNotContain("s3cr3t-value").doesNotContain("a+b%26c");
    assertThat(redactor.headers(Map.of("X-Note", List.of("see s3cr3t-value"))).get("X-Note")).containsExactly("see " + R);
    assertThat((String) redactor.body(bytes("{\"note\":\"has s3cr3t-value in it\"}"), "application/json").get("body")).doesNotContain("s3cr3t-value");
    assertThat((String) redactor.body(bytes("plain s3cr3t-value text"), "text/plain").get("body")).isEqualTo("plain " + R + " text");
    assertThat(redactor.single(Map.of("fileName", "s3cr3t-value.csv")).get("fileName")).isEqualTo(R + ".csv");
  }

  @Test
  void aSecretTooShortToSearchForIsLeftAlone() {
    assertThat(new TraceRedactor(List.of("ab")).text("a ab b")).isEqualTo("a ab b");
  }

  @Test
  void aBodyThatIsNotTextGoesOutAsBase64ItsStartOnly() {
    byte[] binary = new byte[40_000];
    for (int i = 0; i < binary.length; i++) {
      binary[i] = (byte) (0x80 + (i % 64));
    }

    Map<String, Object> out = plain.body(binary, "application/octet-stream");

    assertThat(out.get("bodyEncoding")).isEqualTo("base64");
    assertThat(out.get("bodyBytes")).isEqualTo(40_000L);
    assertThat(out.get("bodyTruncated")).isEqualTo(true);
    assertThat(Base64.getDecoder().decode((String) out.get("body"))).hasSize(TraceRedactor.BODY_LIMIT).startsWith(binary[0], binary[1]);
  }

  @Test
  void textWithANulIsBase64BecauseThePlatformCannotStoreIt() {
    Map<String, Object> out = plain.body(bytes("a\u0000b"), "text/plain");

    assertThat(out.get("bodyEncoding")).isEqualTo("base64");
  }

  @Test
  void aLongTextBodyIsCutAtTheLimitWithoutSplittingACharacter() {
    // Two-byte characters: the cut at 32768 bytes falls between them or in one.
    String text = "é".repeat(20_000);

    Map<String, Object> out = plain.body(bytes(text), "text/plain");

    String kept = (String) out.get("body");
    assertThat(kept.getBytes(StandardCharsets.UTF_8).length).isLessThanOrEqualTo(TraceRedactor.BODY_LIMIT);
    assertThat(kept).matches("é+").hasSize(TraceRedactor.BODY_LIMIT / 2);
    assertThat(out.get("bodyBytes")).isEqualTo(40_000L);
    assertThat(out.get("bodyTruncated")).isEqualTo(true);
  }

  @Test
  void aBodyOfMegabytesIsNotParsedButItsStartIsStillRedacted() {
    String start = "{\"password\":\"hunter2\",\"filler\":\"";
    String huge = start + "x".repeat(2 * 1024 * 1024) + "\"}";

    Map<String, Object> out = plain.body(bytes(huge), "application/json");

    assertThat((String) out.get("body")).doesNotContain("hunter2").contains(R);
    assertThat(out.get("bodyBytes")).isEqualTo((long) huge.length());
    assertThat(out.get("bodyTruncated")).isEqualTo(true);
  }

  @Test
  void noBodyIsANullBodyOfZeroBytes() {
    Map<String, Object> out = plain.body(null, null);

    assertThat(out.get("body")).isNull();
    assertThat(out.get("bodyBytes")).isEqualTo(0L);
    assertThat(out.get("bodyTruncated")).isEqualTo(false);
  }
}
