package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonSubTypes;
import com.fasterxml.jackson.annotation.JsonTypeInfo;
import java.time.Duration;
import java.util.Map;
import org.jspecify.annotations.Nullable;

/**
 * How an API poll logs in to the API it polls. A token is fetched when first needed, kept until
 * shortly before it expires, and fetched again once when the API answers {@code 401} or {@code 403}.
 * Secrets are named, never given.
 */
@JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
@JsonSubTypes({
  @JsonSubTypes.Type(value = ApiAuth.None.class, name = "none"),
  @JsonSubTypes.Type(value = ApiAuth.Basic.class, name = "basic"),
  @JsonSubTypes.Type(value = ApiAuth.Bearer.class, name = "bearer"),
  @JsonSubTypes.Type(value = ApiAuth.ApiKey.class, name = "api-key"),
  @JsonSubTypes.Type(value = ApiAuth.ClientCredentials.class, name = "oauth-client-credentials"),
  @JsonSubTypes.Type(value = ApiAuth.Login.class, name = "login"),
})
public sealed interface ApiAuth {

  /** No credentials. */
  record None() implements ApiAuth {}

  /**
   * HTTP basic authentication.
   *
   * @param username the user name
   * @param password the password
   */
  record Basic(String username, SecretRef password) implements ApiAuth {}

  /**
   * A fixed token, sent as {@code Authorization: Bearer <token>}.
   *
   * @param token the token
   */
  record Bearer(SecretRef token) implements ApiAuth {}

  /**
   * A key in a header of its own, such as {@code X-Api-Key}.
   *
   * @param header the header's name
   * @param value the key
   */
  record ApiKey(String header, SecretRef value) implements ApiAuth {}

  /**
   * An OAuth 2 access token from the client credentials grant, sent as {@code Authorization: Bearer}.
   *
   * @param tokenUrl the token endpoint
   * @param clientId the client id
   * @param clientSecret the client secret
   * @param scope the scope asked for, or null
   * @param audience the audience asked for, or null (some providers require it)
   * @param credentialsIn where the client id and secret are sent
   */
  record ClientCredentials(
      String tokenUrl,
      String clientId,
      SecretRef clientSecret,
      @Nullable String scope,
      @Nullable String audience,
      CredentialsIn credentialsIn)
      implements ApiAuth {

    /** Where the client id and secret are sent. */
    public enum CredentialsIn {
      /** An {@code Authorization: Basic} header (RFC 6749's preference). */
      BASIC_HEADER,
      /** The form body, as {@code client_id} and {@code client_secret}. */
      BODY
    }
  }

  /**
   * A login endpoint of the API's own: a user name and password posted, a token read from the
   * answer and sent on every request.
   *
   * @param url the login endpoint
   * @param format how the login is posted
   * @param usernameField the field the user name goes in, such as {@code username}
   * @param passwordField the field the password goes in, such as {@code password}
   * @param username the user name
   * @param password the password
   * @param extraFields other fields the login needs, such as {@code grant_type=password}; may be empty
   * @param tokenPath where the token is in the answer, such as {@code $.access_token}
   * @param expiresInPath where the answer says how many seconds the token lasts, or null
   * @param lifetime how long a token is kept when the answer does not say
   * @param header the header the token is sent in, usually {@code Authorization}
   * @param prefix put before the token in that header, such as {@code Bearer } (with its space); may be empty
   */
  record Login(
      String url,
      Format format,
      String usernameField,
      String passwordField,
      String username,
      SecretRef password,
      @Nullable Map<String, String> extraFields,
      String tokenPath,
      @Nullable String expiresInPath,
      Duration lifetime,
      String header,
      String prefix)
      implements ApiAuth {

    /** How a login is posted. */
    public enum Format {
      /** {@code application/x-www-form-urlencoded}. */
      FORM,
      /** A JSON object. */
      JSON
    }

    /** The extra fields; none when absent. */
    public Map<String, String> extras() {
      Map<String, String> extras = extraFields;
      return extras == null ? Map.of() : extras;
    }
  }
}
