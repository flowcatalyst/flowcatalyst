package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonSubTypes;
import com.fasterxml.jackson.annotation.JsonTypeInfo;
import org.jspecify.annotations.Nullable;

/** How the caller of an HTTP ingestion authenticates. */
@JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
@JsonSubTypes({
  @JsonSubTypes.Type(value = HttpAuth.FlowCatalystToken.class, name = "flowcatalyst-token"),
  @JsonSubTypes.Type(value = HttpAuth.Basic.class, name = "basic"),
  @JsonSubTypes.Type(value = HttpAuth.Bearer.class, name = "bearer"),
})
public sealed interface HttpAuth {

  /**
   * An access token issued by FlowCatalyst. Its principal must have access to the ingestion's client
   * ({@code clients} holds {@code *} or the client) and to the application ({@code applications}
   * holds {@code *} or the application's code); with a principal id, it must also be that principal.
   *
   * @param application the code of the application the caller must have access to
   * @param principalId when set, the only principal accepted ({@code prn_...}, the token's {@code
   *     sub}): for a service account, its principal's id, not its {@code sac_} id, which its tokens
   *     do not carry
   */
  record FlowCatalystToken(String application, @Nullable String principalId) implements HttpAuth {}

  /**
   * HTTP basic authentication.
   *
   * @param username the expected user name
   * @param password the expected password
   */
  record Basic(String username, SecretRef password) implements HttpAuth {}

  /**
   * A fixed bearer token.
   *
   * @param token the expected token
   */
  record Bearer(SecretRef token) implements HttpAuth {}
}
